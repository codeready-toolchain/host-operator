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
	spaceProvisionerConfigPredicate func(*toolchainv1alpha1.SpaceProvisionerConfig) bool

	// SpaceCountGetter is a function useful for mocking the counts cache and can be passed
	// to the NewClusterManager function. The returned tuple represents the actual number
	// of the spaces found for given cluster and the fact whether the value was actually found or not
	// (similar to the return value of the map lookup).
	SpaceCountGetter func(ctx context.Context, clusterName string) (int, bool)
)

// Even though the readiness status of the SpaceProvisionerConfig is based (in part) on the space count
// we do another check here using the fresh data from the counts cache. This is to ensure we don't
// overcommit spaces to the clusters.
// The returned predicate assumes that the incoming SPC has already been updated with the fresh value
// from the counts cache.
func checkHasNotReachedSpaceCountThreshold() spaceProvisionerConfigPredicate {
	return func(spc *toolchainv1alpha1.SpaceProvisionerConfig) bool {
		var spaceCount int
		if spc.Status.ConsumedCapacity != nil {
			spaceCount = spc.Status.ConsumedCapacity.SpaceCount
		}

		threshold := spc.Spec.CapacityThresholds.MaxNumberOfSpaces

		return threshold == 0 || uint(spaceCount) < threshold // nolint:gosec // we're not gonna overflow with the number of spaces
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

func DefaultClusterManager(namespace string, cl runtimeclient.Client) *ClusterManager {
	return NewClusterManager(namespace, cl, nil)
}

// NewClusterManager constructs a new cluster manager for the namespace using the provided client.
// If the `getSpaceCount` is nil, the default behavior is to find the space count in the counts cache.
//
// In tests this can be replaced with
// github.com/codeready-toolchain/host-operator/test/spaceprovisionerconfig.GetSpaceCountFromSpaceProvisionerConfigs
// function that reads the counts solely from the configured SPCs so that the counts cache and ToolchainStatus
// doesn't have to be constructed and initialized making the setup of the tests slightly easier.
func NewClusterManager(namespace string, cl runtimeclient.Client, getSpaceCount SpaceCountGetter) *ClusterManager {
	return &ClusterManager{
		namespace:     namespace,
		client:        cl,
		getSpaceCount: getSpaceCount,
	}
}

type ClusterManager struct {
	getSpaceCount SpaceCountGetter
	namespace     string
	client        runtimeclient.Client
	lastUsed      string
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
	spaceCountGetter, err := b.getSpaceCountGetter()
	if err != nil {
		return "", fmt.Errorf("failed to get the function to obtain the space counts: %w", err)
	}

	optimalSpaceProvisioners, err := b.getOptimalTargetClusters(
		ctx,
		optimalClusterFilter.PreferredCluster,
		spaceCountGetter,
		isReady(),
		checkHasNotReachedSpaceCountThreshold(),
		hasPlacementRoles(optimalClusterFilter.ClusterRoles))
	if err != nil {
		return "", fmt.Errorf("failed to find the optimal space provisioner config: %w", err)
	}

	// after the above function call, we will have the candidate SPCs which are also updated with the latest
	// stats from the cache. We can therefore only use the SPCs in the code below.

	if len(optimalSpaceProvisioners) == 0 {
		return "", nil
	}

	if len(optimalSpaceProvisioners) == 1 {
		return optimalSpaceProvisioners[0].Spec.ToolchainCluster, nil
	}

	for _, spc := range optimalSpaceProvisioners {
		spc := spc
		clusterName := spc.Spec.ToolchainCluster
		if clusterName == b.lastUsed {
			provisioned := getConsumedSpaceCount(&spc)
			if provisioned%50 != 0 {
				return clusterName, nil
			}
		}
	}

	sort.Slice(optimalSpaceProvisioners, func(i, j int) bool {
		spc1 := optimalSpaceProvisioners[i]
		provisioned1 := getConsumedSpaceCount(&spc1)
		threshold1 := spc1.Spec.CapacityThresholds.MaxNumberOfSpaces

		spc2 := optimalSpaceProvisioners[j]
		provisioned2 := getConsumedSpaceCount(&spc2)
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

func (b *ClusterManager) getSpaceCountGetter() (SpaceCountGetter, error) {
	if b.getSpaceCount != nil {
		return b.getSpaceCount, nil
	}

	counts, err := counter.GetCounts()
	if err != nil {
		return nil, err
	}

	return func(_ context.Context, clusterName string) (int, bool) {
		count, ok := counts.SpacesPerClusterCounts[clusterName]
		return count, ok
	}, nil
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
// The function returns a slice of matching SpaceProvisionerConfigs. If there are no matches, the empty slice is represented by a nil value (which is the default value in Go).
// The returned SpaceProvisionerConfigs have their space counts updated from the counts cache and therefore can have a more recent information about the space count than what's
// actually persisted in the cluster at the moment.
func (b *ClusterManager) getOptimalTargetClusters(ctx context.Context, preferredCluster string, spaceCountGetter SpaceCountGetter, predicates ...spaceProvisionerConfigPredicate) ([]toolchainv1alpha1.SpaceProvisionerConfig, error) {
	list := &toolchainv1alpha1.SpaceProvisionerConfigList{}
	if err := b.client.List(ctx, list, runtimeclient.InNamespace(b.namespace)); err != nil {
		return nil, err
	}

	matching := make([]toolchainv1alpha1.SpaceProvisionerConfig, 0, len(list.Items))

	for _, spc := range list.Items {
		spc := spc
		updateSpaceCountFromCache(ctx, spaceCountGetter, &spc)
		if matches(&spc, predicates) {
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

func updateSpaceCountFromCache(ctx context.Context, getSpaceCount SpaceCountGetter, spc *toolchainv1alpha1.SpaceProvisionerConfig) {
	if spaceCount, ok := getSpaceCount(ctx, spc.Spec.ToolchainCluster); ok {
		// the cached spaceCount is always going to be fresher than (or as fresh as) what's in the SPC
		if spc.Status.ConsumedCapacity == nil {
			spc.Status.ConsumedCapacity = &toolchainv1alpha1.ConsumedCapacity{}
		}
		spc.Status.ConsumedCapacity.SpaceCount = spaceCount
	}
}

func getConsumedSpaceCount(spc *toolchainv1alpha1.SpaceProvisionerConfig) int {
	if spc.Status.ConsumedCapacity == nil {
		return 0
	}
	return spc.Status.ConsumedCapacity.SpaceCount
}
