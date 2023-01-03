package capacity

import (
	"context"
	"sort"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func hasNotReachedMaxNumberOfSpacesThreshold(config toolchainconfig.ToolchainConfig, counts counter.Counts) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		numberOfSpaces := counts.SpacesPerClusterCounts[cluster.Name]
		threshold := config.CapacityThresholds().MaxNumberOfSpacesSpecificPerMemberCluster()[cluster.Name]
		return threshold == 0 || numberOfSpaces < threshold
	}
}

func hasEnoughResources(config toolchainconfig.ToolchainConfig, status *toolchainv1alpha1.ToolchainStatus) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		threshold, found := config.CapacityThresholds().ResourceCapacityThresholdSpecificPerMemberCluster()[cluster.Name]
		if !found {
			threshold = config.CapacityThresholds().ResourceCapacityThresholdDefault()
		}
		if threshold == 0 {
			return true
		}
		for _, memberStatus := range status.Status.Members {
			if memberStatus.ClusterName == cluster.Name {
				return hasMemberREnoughResources(memberStatus, threshold)
			}
		}
		return false
	}
}

func hasMemberREnoughResources(memberStatus toolchainv1alpha1.Member, threshold int) bool {
	if len(memberStatus.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole) > 0 {
		for _, usagePerNode := range memberStatus.MemberStatus.ResourceUsage.MemoryUsagePerNodeRole {
			if usagePerNode >= threshold {
				return false
			}
		}
		return true
	}
	return false
}

func NewClusterManager(getMemberClusters cluster.GetMemberClustersFunc, cl client.Client) *ClusterManager {
	return &ClusterManager{
		getMemberClusters: getMemberClusters,
		client:            cl,
	}
}

type ClusterManager struct {
	getMemberClusters cluster.GetMemberClustersFunc
	client            client.Client
	lastUsed          string
}

// GetOptimalTargetCluster returns the name of the cluster with the most available capacity where a Space could be provisioned.
//
// If two clusters have the same limit and they both have the same usage, then the logic distributes spaces in a batches of 50.
//
// If the two clusters don't have the same limit, then the batch is based on the scale of the limits.
// Let's say that the limit for member1 is 1000 and for member2 is 2000, then the batch of spaces would be 50 for member1 and 100 for member2.
//
// If the preferredCluster is provided and it is also one of the available clusters, then the same name is returned.
func (b *ClusterManager) GetOptimalTargetCluster(preferredCluster, namespace string) (string, error) {
	config, err := toolchainconfig.GetToolchainConfig(b.client)
	if err != nil {
		return "", errors.Wrapf(err, "unable to get ToolchainConfig")
	}

	counts, err := counter.GetCounts()
	if err != nil {
		return "", errors.Wrapf(err, "unable to get the number of provisioned spaces")
	}

	status := &toolchainv1alpha1.ToolchainStatus{}
	if err := b.client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: toolchainconfig.ToolchainStatusName}, status); err != nil {
		return "", errors.Wrapf(err, "unable to read ToolchainStatus resource")
	}
	optimalTargetClusters := getOptimalTargetClusters(preferredCluster, b.getMemberClusters, hasNotReachedMaxNumberOfSpacesThreshold(config, counts), hasEnoughResources(config, status))

	for _, cluster := range optimalTargetClusters {
		if cluster == b.lastUsed {
			provisioned := counts.SpacesPerClusterCounts[cluster]
			if provisioned%50 != 0 {
				return cluster, nil
			}
		}
	}

	sort.Slice(optimalTargetClusters, func(i, j int) bool {
		provisioned1 := counts.SpacesPerClusterCounts[optimalTargetClusters[i]]
		threshold1 := config.CapacityThresholds().MaxNumberOfSpacesSpecificPerMemberCluster()[optimalTargetClusters[i]]

		provisioned2 := counts.SpacesPerClusterCounts[optimalTargetClusters[j]]
		threshold2 := config.CapacityThresholds().MaxNumberOfSpacesSpecificPerMemberCluster()[optimalTargetClusters[j]]

		// Let's round the number of provisioned users down to the closest multiple of 50
		// This is a trick we need to do before comparing the capacity, so we can distribute the users in batches by 50 (if the clusters have the same limit)
		provisioned1By50 := (provisioned1 / 50) * 50
		provisioned2By50 := (provisioned2 / 50) * 50

		// now we can calculate what is the actual usage of the clusters (how many users are provisioned there compared to the threshold) and compare them
		return float64(provisioned1By50)/float64(threshold1) < float64(provisioned2By50)/float64(threshold2)
	})

	b.lastUsed = optimalTargetClusters[0]
	return optimalTargetClusters[0], nil
}

func getOptimalTargetClusters(preferredCluster string, getMemberClusters cluster.GetMemberClustersFunc, conditions ...cluster.Condition) []string {
	// Automatic cluster selection based on cluster readiness
	members := getMemberClusters(append(conditions, cluster.Ready)...)
	if len(members) == 0 {
		return []string{""}
	}

	// if the preferred cluster is provided and it is also one of the available clusters, then the same name is returned, otherwise, it returns the first available one
	if preferredCluster != "" {
		for _, m := range members {
			if preferredCluster == m.Name {
				return []string{m.Name}
			}
		}
	}

	memberNames := make([]string, len(members))
	for i := range members {
		memberNames[i] = members[i].Name
	}
	return memberNames
}
