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

func hasNotReachedMaxNumberOfUsersThreshold(config toolchainconfig.ToolchainConfig, counts counter.Counts) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		if config.AutomaticApproval().MaxNumberOfUsersOverall() != 0 {
			if config.AutomaticApproval().MaxNumberOfUsersOverall() <= (counts.MasterUserRecords()) {
				return false
			}
		}
		numberOfUserAccounts := counts.UserAccountsPerClusterCounts[cluster.Name]
		threshold := config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster()[cluster.Name]
		return threshold == 0 || numberOfUserAccounts < threshold
	}
}

func hasEnoughResources(config toolchainconfig.ToolchainConfig, status *toolchainv1alpha1.ToolchainStatus) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		threshold, found := config.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster()[cluster.Name]
		if !found {
			threshold = config.AutomaticApproval().ResourceCapacityThresholdDefault()
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

// GetOptimalTargetCluster returns the name of the cluster with the most available capacity where a Space could be provisioned.
// If the preferredCluster is provided and it is also one of the available clusters, then the same name is returned.
func GetOptimalTargetCluster(preferredCluster, namespace string, getMemberClusters cluster.GetMemberClustersFunc, cl client.Client) (string, error) {
	config, err := toolchainconfig.GetToolchainConfig(cl)
	if err != nil {
		return "", errors.Wrapf(err, "unable to get ToolchainConfig")
	}

	counts, err := counter.GetCounts()
	if err != nil {
		return "", errors.Wrapf(err, "unable to get the number of provisioned users")
	}

	status := &toolchainv1alpha1.ToolchainStatus{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: toolchainconfig.ToolchainStatusName}, status); err != nil {
		return "", errors.Wrapf(err, "unable to read ToolchainStatus resource")
	}
	optimalTargetClusters := getOptimalTargetClusters(preferredCluster, getMemberClusters, hasNotReachedMaxNumberOfUsersThreshold(config, counts), hasEnoughResources(config, status))

	sort.Slice(optimalTargetClusters, func(i, j int) bool {
		provisioned1 := counts.UserAccountsPerClusterCounts[optimalTargetClusters[i]]
		threshold1 := config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster()[optimalTargetClusters[i]]

		provisioned2 := counts.UserAccountsPerClusterCounts[optimalTargetClusters[j]]
		threshold2 := config.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster()[optimalTargetClusters[j]]

		return float32(provisioned1)/float32(threshold1) < float32(provisioned2)/float32(threshold2)
	})
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
