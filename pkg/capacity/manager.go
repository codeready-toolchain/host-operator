package capacity

import (
	"context"
	"fmt"
	"sort"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type (
	provisionerCandidate struct {
		clusterName         string
		spaceCount          int
		spaceCountThreshold int
		isReady             bool
		placementRoles      []string
	}

	provisionerPredicate func(provisionerCandidate) bool
)

func checkHasNotReachedSpaceCountThreshold() provisionerPredicate {
	return func(candidate provisionerCandidate) bool {
		spaceCount := candidate.spaceCount
		threshold := candidate.spaceCountThreshold

		return threshold == 0 || spaceCount < threshold
	}
}

func isReady() provisionerPredicate {
	return func(candidate provisionerCandidate) bool {
		return candidate.isReady
	}
}

func hasPlacementRoles(requiredPlacementRoles []string) provisionerPredicate {
	if len(requiredPlacementRoles) == 0 {
		// by default it should pick the `tenant` placement role, if no specific placement role was provided
		requiredPlacementRoles = []string{cluster.RoleLabel(cluster.Tenant)}
	}

	return func(candidate provisionerCandidate) bool {
		// filter member cluster having the required placement role
	placementCheck:
		for _, requiredPlacement := range requiredPlacementRoles {
			for _, placement := range candidate.placementRoles {
				if placement == requiredPlacement {
					continue placementCheck
				}
			}
			return false
		}

		// all placement roles were matched
		return true
	}
}

// NewClusterManager constructs a new cluster manager for the namespace using the provided client.
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
	counts, err := counter.GetSpaceCountPerClusterSnapshot()
	if err != nil {
		return "", fmt.Errorf("failed to obtain the counts cache: %w", err)
	}

	// NOTE: the isReady(), checkHasNotReachedSpaceCountThreshold() combination of predicates is not perfect and we only use it
	// to prevent OVER-commitment of spaces to clusters. We do not guarantee that UNDER-commitment doesn't happen.
	//
	// isReady() simply checks the ready status of the SPC which might be based on slightly out-of-date information if there were changes
	// in the space count or memory utilization of the cluster between the last reconcile of the SPC (which happens on every change of ToolchainStatus)
	// and now.
	//
	// So if the SPC is ready (meaning it was underutilized at the last SPC reconcile) but now has breached the space capacity, we can detect that
	// situation and prevent over-commitment.
	//
	// If the SPC was not ready (due to space count or memory utilization reaching the capacity) but is now below the threshold, we DO NOT try to rectify
	// the result here and still consider such cluster unavailable. Instead, we just wait until the change is propagated through the ToolchainStatus
	// into the SPC's status.
	//
	// This means that we prevent only the over-commitment of spaces. We DO NOT prevent breaching the memory capacity in 100% of the cases (it fluctuates
	// a lot anyway) and we DO NOT guarantee that a space will be deployed to a cluster that has only very recently dropped under its utilization capacity.
	candidates, err := b.getOptimalTargetClusters(
		ctx,
		optimalClusterFilter.PreferredCluster,
		counts,
		isReady(),
		checkHasNotReachedSpaceCountThreshold(),
		hasPlacementRoles(optimalClusterFilter.ClusterRoles))
	if err != nil {
		return "", fmt.Errorf("failed to find the optimal space provisioner config: %w", err)
	}

	// after the above function call, we will have the candidate SPCs which are also updated with the latest
	// stats from the cache. We can therefore only use the SPCs in the code below.

	if len(candidates) == 0 {
		return "", nil
	}

	if len(candidates) == 1 {
		return candidates[0].clusterName, nil
	}

	for _, candidate := range candidates {
		candidate := candidate
		clusterName := candidate.clusterName
		if clusterName == b.lastUsed {
			provisioned := candidate.spaceCount
			if provisioned%50 != 0 {
				return clusterName, nil
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		candidate1 := candidates[i]
		provisioned1 := candidate1.spaceCount
		threshold1 := candidate1.spaceCountThreshold

		candidate2 := candidates[j]
		provisioned2 := candidate2.spaceCount
		threshold2 := candidate2.spaceCountThreshold

		// Let's round the number of provisioned users down to the closest multiple of 50
		// This is a trick we need to do before comparing the capacity, so we can distribute the users in batches by 50 (if the clusters have the same limit)
		provisioned1By50 := (provisioned1 / 50) * 50
		provisioned2By50 := (provisioned2 / 50) * 50

		// now we can calculate what is the actual usage of the clusters (how many users are provisioned there compared to the threshold) and compare them
		return float64(provisioned1By50)/float64(threshold1) < float64(provisioned2By50)/float64(threshold2)
	})

	b.lastUsed = candidates[0].clusterName
	return b.lastUsed, nil
}

func matches(candidate provisionerCandidate, predicates []provisionerPredicate) bool {
	for _, p := range predicates {
		if !p(candidate) {
			return false
		}
	}

	return true
}

// getOptimalTargetClusters checks if a preferred target cluster was provided and available from the cluster pool.
// If the preferred target cluster was not provided or not available, but a list of clusterRoles was provided, then it filters only the available clusters matching all those roles.
// If no cluster roles were provided then it returns all the available clusters.
// The function returns a slice of matching SpaceProvisionerConfigs. If there are no matches, the empty slice is represented by a nil value (which is the default value in Go).
// The returned SpaceProvisionerConfigs have their space counts updated from the counts cache and therefore can have a more recent information about the space count than what's
// actually persisted in the cluster at the moment.
func (b *ClusterManager) getOptimalTargetClusters(ctx context.Context, preferredCluster string, counts map[string]int, predicates ...provisionerPredicate) ([]provisionerCandidate, error) {
	list := &toolchainv1alpha1.SpaceProvisionerConfigList{}
	if err := b.client.List(ctx, list, runtimeclient.InNamespace(b.namespace)); err != nil {
		return nil, err
	}

	matching := make([]provisionerCandidate, 0, len(list.Items))

	for _, spc := range list.Items {
		spc := spc // to prevent the memory aliasing in a for-loop
		candidate := provisionerCandidateFromSPC(&spc, counts)
		if matches(candidate, predicates) {
			matching = append(matching, candidate)
		}
	}

	if len(matching) == 0 {
		return nil, nil
	}

	// if the preferred cluster is provided and it is also one of the available clusters, then the same name is returned, otherwise, it returns the first available one
	if preferredCluster != "" {
		for _, candidate := range matching {
			if preferredCluster == candidate.clusterName {
				return []provisionerCandidate{candidate}, nil
			}
		}
	}

	// return the member names in case some were found
	return matching, nil
}

func provisionerCandidateFromSPC(spc *toolchainv1alpha1.SpaceProvisionerConfig, counts map[string]int) provisionerCandidate {
	return provisionerCandidate{
		clusterName:         spc.Spec.ToolchainCluster,
		spaceCount:          counts[spc.Spec.ToolchainCluster],
		spaceCountThreshold: int(spc.Spec.CapacityThresholds.MaxNumberOfSpaces), //nolint:gosec // this just doesn't overflow there's no way of having > 2*10^9 namespaces in a cluster
		isReady:             condition.IsTrue(spc.Status.Conditions, toolchainv1alpha1.ConditionReady),
		placementRoles:      spc.Spec.PlacementRoles,
	}
}
