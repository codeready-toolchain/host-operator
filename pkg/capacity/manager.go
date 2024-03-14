package capacity

import (
	"context"
	"fmt"
	"sort"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"

	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type (
	spaceProvisionerConfigPredicate func(*toolchainv1alpha1.SpaceProvisionerConfig) bool
)

func hasNotReachedMaxNumberOfSpacesThreshold(counts counter.Counts) spaceProvisionerConfigPredicate {
	return func(spc *toolchainv1alpha1.SpaceProvisionerConfig) bool {
		numberOfSpaces := uint(counts.SpacesPerClusterCounts[spc.Spec.ToolchainCluster])
		threshold := spc.Spec.CapacityThresholds.MaxNumberOfSpaces
		return threshold == 0 || numberOfSpaces < threshold
	}
}

func hasEnoughMemoryCapacity(status *toolchainv1alpha1.ToolchainStatus) spaceProvisionerConfigPredicate {
	return func(spc *toolchainv1alpha1.SpaceProvisionerConfig) bool {
		threshold := spc.Spec.CapacityThresholds.MaxMemoryUtilizationPercent
		if threshold == 0 {
			return true
		}
		for _, memberStatus := range status.Status.Members {
			if memberStatus.ClusterName == spc.Spec.ToolchainCluster {
				return hasMemberEnoughMemoryCapacity(memberStatus, threshold)
			}
		}
		return false
	}
}

func hasMemberEnoughMemoryCapacity(memberStatus toolchainv1alpha1.Member, threshold uint) bool {
	if len(memberStatus.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole) > 0 {
		for _, usagePerNode := range memberStatus.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole {
			if uint(usagePerNode) >= threshold {
				return false
			}
		}
		return true
	}
	return false
}

func isProvisioningEnabled() spaceProvisionerConfigPredicate {
	return func(spc *toolchainv1alpha1.SpaceProvisionerConfig) bool {
		return spc.Spec.Enabled
	}
}

func isReady() spaceProvisionerConfigPredicate {
	return func(spc *toolchainv1alpha1.SpaceProvisionerConfig) bool {
		return condition.IsTrue(spc.Status.Conditions, toolchainv1alpha1.ConditionReady)
	}
}

func hasPlacementRoles(placementRoles []string) spaceProvisionerConfigPredicate {
	return func(spc *toolchainv1alpha1.SpaceProvisionerConfig) bool {
		if len(placementRoles) == 0 {
			// by default it should pick the `tenant` placement role, if no specific placement role was provided
			placementRoles = []string{cluster.RoleLabel(cluster.Tenant)}
		}

		// filter member cluster having the required placement role
	placementCheck:
		for _, placement := range placementRoles {
			for _, requiredPlacement := range spc.Spec.PlacementRoles {
				if requiredPlacement == placement {
					continue placementCheck
				}
			}
			return false
		}

		// all placement roles were matched
		return true
	}
}

func NewClusterManager(namespace string, cl runtimeclient.Client) *ClusterManager {
	return &ClusterManager{
		namespace: namespace,
		client:    cl,
	}
}

type ClusterManager struct {
	namespace string
	client    runtimeclient.Client
	lastUsed  string
}

// OptimalTargetClusterFilter is used by GetOptimalTargetCluster
// in order to retrieve an "optimal" cluster for the Space to be provisioned into.
type OptimalTargetClusterFilter struct {
	// PreferredCluster if specified and available,
	// it will be used to find the desired member cluster by name.
	PreferredCluster string
	// ToolchainStatusNamespace is the namespace where the toolchainstatus CR will be searched,
	// in order to check which cluster has enough resources and can be candidate for the "optimal" cluster for Space provisioning.
	ToolchainStatusNamespace string
	// ClusterRoles is a list of cluster-role labels,
	// if provided, only the clusters matching those labels will be selected as candidates for the "optimal" cluster.
	ClusterRoles []string
}

// GetOptimalTargetCluster returns the name of the cluster with the most available capacity where a Space could be provisioned.
//
// If two clusters have the same limit and they both have the same usage, then the logic distributes spaces in a batches of 50.
//
// If the two clusters don't have the same limit, then the batch is based on the scale of the limits.
// Let's say that the limit for member1 is 1000 and for member2 is 2000, then the batch of spaces would be 50 for member1 and 100 for member2.
//
// If the preferredCluster is provided and it is also one of the available clusters, then the same name is returned.
// In case the preferredCluster was not provided or not found/available and the clusterRoles are provided then the candidates optimal cluster pool will be made out by only those matching the labels, if any available.
func (b *ClusterManager) GetOptimalTargetCluster(ctx context.Context, optimalClusterFilter OptimalTargetClusterFilter) (string, error) {
	counts, err := counter.GetCounts()
	if err != nil {
		return "", fmt.Errorf("unable to get the number of provisioned spaces: %w", err)
	}

	status := &toolchainv1alpha1.ToolchainStatus{}
	if err := b.client.Get(ctx, types.NamespacedName{Namespace: optimalClusterFilter.ToolchainStatusNamespace, Name: toolchainconfig.ToolchainStatusName}, status); err != nil {
		return "", fmt.Errorf("unable to read ToolchainStatus resource: %w", err)
	}
	optimalSpaceProvisioners, err := b.getOptimalTargetClusters(
		ctx,
		optimalClusterFilter.PreferredCluster,
		isReady(),
		isProvisioningEnabled(),
		hasPlacementRoles(optimalClusterFilter.ClusterRoles),
		hasNotReachedMaxNumberOfSpacesThreshold(counts),
		hasEnoughMemoryCapacity(status))
	if err != nil {
		return "", fmt.Errorf("failed to find the optimal space provisioner config: %w", err)
	}

	if len(optimalSpaceProvisioners) == 0 {
		return "", nil
	}

	if len(optimalSpaceProvisioners) == 1 {
		return optimalSpaceProvisioners[0].Spec.ToolchainCluster, nil
	}

	for _, spc := range optimalSpaceProvisioners {
		clusterName := spc.Spec.ToolchainCluster
		if clusterName == b.lastUsed {
			provisioned := counts.SpacesPerClusterCounts[clusterName]
			if provisioned%50 != 0 {
				return clusterName, nil
			}
		}
	}

	sort.Slice(optimalSpaceProvisioners, func(i, j int) bool {
		spc1 := optimalSpaceProvisioners[i]
		cluster1 := spc1.Spec.ToolchainCluster
		provisioned1 := counts.SpacesPerClusterCounts[cluster1]
		threshold1 := spc1.Spec.CapacityThresholds.MaxNumberOfSpaces

		spc2 := optimalSpaceProvisioners[j]
		cluster2 := spc2.Spec.ToolchainCluster
		provisioned2 := counts.SpacesPerClusterCounts[cluster2]
		threshold2 := spc2.Spec.CapacityThresholds.MaxNumberOfSpaces

		// Let's round the number of provisioned users down to the closest multiple of 50
		// This is a trick we need to do before comparing the capacity, so we can distribute the users in batches by 50 (if the clusters have the same limit)
		provisioned1By50 := (provisioned1 / 50) * 50
		provisioned2By50 := (provisioned2 / 50) * 50

		// now we can calculate what is the actual usage of the clusters (how many users are provisioned there compared to the threshold) and compare them
		return float64(provisioned1By50)/float64(threshold1) < float64(provisioned2By50)/float64(threshold2)
	})

	b.lastUsed = optimalSpaceProvisioners[0].Spec.ToolchainCluster
	return b.lastUsed, nil
}

func matches(spc *toolchainv1alpha1.SpaceProvisionerConfig, predicates []spaceProvisionerConfigPredicate) bool {
	for _, p := range predicates {
		if !p(spc) {
			return false
		}
	}

	return true
}

// getOptimalTargetClusters checks if a preferred target cluster was provided and available from the cluster pool.
// If the preferred target cluster was not provided or not available, but a list of clusterRoles was provided, then it filters only the available clusters matching all those roles.
// If no cluster roles were provided then it returns all the available clusters.
// The function returns a slice with an empty string if not optimal target clusters where found.
func (b *ClusterManager) getOptimalTargetClusters(ctx context.Context, preferredCluster string, conditions ...spaceProvisionerConfigPredicate) ([]toolchainv1alpha1.SpaceProvisionerConfig, error) {
	list := &toolchainv1alpha1.SpaceProvisionerConfigList{}
	if err := b.client.List(ctx, list, runtimeclient.InNamespace(b.namespace)); err != nil {
		return nil, err
	}

	matching := make([]toolchainv1alpha1.SpaceProvisionerConfig, 0, len(list.Items))

	for _, spc := range list.Items {
		spc := spc
		if matches(&spc, conditions) {
			matching = append(matching, spc)
		}
	}

	if len(matching) == 0 {
		return nil, nil
	}

	// if the preferred cluster is provided and it is also one of the available clusters, then the same name is returned, otherwise, it returns the first available one
	if preferredCluster != "" {
		for _, member := range matching {
			if preferredCluster == member.Spec.ToolchainCluster {
				return []toolchainv1alpha1.SpaceProvisionerConfig{member}, nil
			}
		}
	}

	// return the member names in case some were found
	return matching, nil
}
