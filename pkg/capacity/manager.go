package capacity

import (
	"context"
	"fmt"
	"sort"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"

	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func hasNotReachedMaxNumberOfSpacesThreshold(ctx context.Context, counts counter.Counts) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		numberOfSpaces := counts.SpacesPerClusterCounts[cluster.Name]
		threshold := cluster.Provisioning.CapacityThresholds.MaxNumberOfSpaces
		if threshold == 0 || uint(numberOfSpaces) < threshold {
			log.FromContext(ctx).Info("------------------------------------------- nof spaces threshold passed", "counts", counts, "threshold", threshold)
			return true
		} else {
			log.FromContext(ctx).Info("------------------------------------------- nof spaces threshold didn't pass", "counts", counts, "threshold", threshold)
			return false
		}
	}
}

func hasEnoughMemoryCapacity(ctx context.Context, config *toolchainconfig.ToolchainConfig, status *toolchainv1alpha1.ToolchainStatus) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		threshold := cluster.Provisioning.CapacityThresholds.MaxMemoryUtilizationPercent
		if threshold == 0 {
			log.FromContext(ctx).Info("--------------------------------------- no explicit memory util threshold", "config", config, "status", status)
			threshold = uint(config.CapacityThresholds().ResourceCapacityThresholdDefault())
		}
		if threshold == 0 {
			log.FromContext(ctx).Info("--------------------------------------- even the default max memory util is zero", "config", config, "status", status)
			return true
		}
		for _, memberStatus := range status.Status.Members {
			if memberStatus.ClusterName == cluster.Name {
				return hasMemberEnoughMemoryCapacity(ctx, memberStatus, threshold)
			}
		}
		log.FromContext(ctx).Info("--------------------------------------- no record of the memory usage in the member status", "config", config, "status", status)
		return false
	}
}

func hasMemberEnoughMemoryCapacity(ctx context.Context, memberStatus toolchainv1alpha1.Member, threshold uint) bool {
	if len(memberStatus.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole) > 0 {
		for _, usagePerNode := range memberStatus.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole {
			// negative value is suspicious (and should never happen in practice), so let's ignore the members
			// that report such negative usage.
			if usagePerNode < 0 {
				log.FromContext(ctx).Info("member reports negative memory usage", "member", memberStatus.ClusterName, "usage", usagePerNode)
			}
			if usagePerNode < 0 || uint(usagePerNode) >= threshold {
				return false
			}
		}
		log.FromContext(ctx).Info("------------------------------------------- memory capacity below threshold, yay!", "memberStatus", memberStatus, "threshold", threshold)
		return true
	}
	log.FromContext(ctx).Info("------------------------------------------- no per node memory usage in member status", "memberStatus", memberStatus, "threshold", threshold)
	return false
}

func NewClusterManager(getMemberClusters cluster.GetMemberClustersFunc, cl runtimeclient.Client) *ClusterManager {
	return &ClusterManager{
		getMemberClusters: getMemberClusters,
		client:            cl,
	}
}

type ClusterManager struct {
	getMemberClusters cluster.GetMemberClustersFunc
	client            runtimeclient.Client
	lastUsed          string
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
	// PlacementRoles is a list of placement roles that the clusters need to possess in their SpaceProvisionerConfig to be considered
	// as candidates for the "optimal" cluster. If empty, the default "tenant" role is assumed.
	PlacementRoles []string
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
	config, err := toolchainconfig.GetToolchainConfig(b.client)
	if err != nil {
		return "", fmt.Errorf("unable to get ToolchainConfig: %w", err)
	}

	counts, err := counter.GetCounts()
	if err != nil {
		return "", fmt.Errorf("unable to get the number of provisioned spaces: %w", err)
	}

	status := &toolchainv1alpha1.ToolchainStatus{}
	if err := b.client.Get(ctx, types.NamespacedName{Namespace: optimalClusterFilter.ToolchainStatusNamespace, Name: toolchainconfig.ToolchainStatusName}, status); err != nil {
		return "", fmt.Errorf("unable to read ToolchainStatus resource: %w", err)
	}
	optimalTargetClusters := b.getOptimalTargetClusters(ctx, counts, optimalClusterFilter, &config, status)

	switch len(optimalTargetClusters) {
	case 0:
		{
			log.FromContext(ctx).Info("------------------------------------------ no optimal target cluster found")
			return "", nil
		}
	case 1:
		{
			log.FromContext(ctx).Info("------------------------------------------ a single optimal target cluster found", "cluster", optimalTargetClusters[0].Name)
			return optimalTargetClusters[0].Name, nil
		}
	}

	for _, cluster := range optimalTargetClusters {
		if cluster.Name == b.lastUsed {
			provisioned := counts.SpacesPerClusterCounts[cluster.Name]
			if provisioned%50 != 0 {
				log.FromContext(ctx).Info("------------------------------------------ target cluster picked based on number of provisioned spaces in last used", "cluster", cluster.Name)
				return cluster.Name, nil
			}
		}
	}

	sort.Slice(optimalTargetClusters, func(i, j int) bool {
		provisioned1 := counts.SpacesPerClusterCounts[optimalTargetClusters[i].Name]
		threshold1 := optimalTargetClusters[i].Provisioning.CapacityThresholds.MaxNumberOfSpaces

		provisioned2 := counts.SpacesPerClusterCounts[optimalTargetClusters[j].Name]
		threshold2 := optimalTargetClusters[j].Provisioning.CapacityThresholds.MaxNumberOfSpaces

		// Let's round the number of provisioned users down to the closest multiple of 50
		// This is a trick we need to do before comparing the capacity, so we can distribute the users in batches by 50 (if the clusters have the same limit)
		provisioned1By50 := (provisioned1 / 50) * 50
		provisioned2By50 := (provisioned2 / 50) * 50

		// now we can calculate what is the actual usage of the clusters (how many users are provisioned there compared to the threshold) and compare them
		return float64(provisioned1By50)/float64(threshold1) < float64(provisioned2By50)/float64(threshold2)
	})

	log.FromContext(ctx).Info("------------------------------------------ picked the least used cluster", "cluster", optimalTargetClusters[0].Name)

	b.lastUsed = optimalTargetClusters[0].Name
	return optimalTargetClusters[0].Name, nil
}

// getOptimalTargetClusters checks if a preferred target cluster was provided and available from the cluster pool.
// If the preferred target cluster was not provided or not available, but a list of clusterRoles was provided, then it filters only the available clusters matching all those roles.
// If no cluster roles were provided then it returns all the available clusters.
// The function returns an empty slice if not optimal target clusters where found.
func (b *ClusterManager) getOptimalTargetClusters(ctx context.Context, counts counter.Counts, filter OptimalTargetClusterFilter, config *toolchainconfig.ToolchainConfig, status *toolchainv1alpha1.ToolchainStatus) []*cluster.CachedToolchainCluster {
	members := b.getMemberClusters(
		cluster.Ready,
		cluster.ProvisioningEnabled,
		hasPlacementRoles(filter.PlacementRoles),
		hasNotReachedMaxNumberOfSpacesThreshold(ctx, counts),
		hasEnoughMemoryCapacity(ctx, config, status),
	)
	if filter.PreferredCluster != "" {
		for _, c := range members {
			if c.Name == filter.PreferredCluster {
				return []*cluster.CachedToolchainCluster{c}
			}
		}
	}
	return members
}

func hasPlacementRoles(placementRoles []string) cluster.Condition {
	if len(placementRoles) == 0 {
		// by default it should pick the `tenant` placement role, if no specific placement role was provided
		placementRoles = []string{cluster.RoleLabel(cluster.Tenant)}
	}
	return cluster.WithPlacementRoles(placementRoles...)
}

func hasPreferredName(preferredName string) cluster.Condition {
	return func(clstr *cluster.CachedToolchainCluster) bool {
		return preferredName == "" || clstr.Name == preferredName
	}
}
