package usersignup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	crtCfg "github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/hostoperatorconfig"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type targetCluster string

var (
	unknown  targetCluster = "unknown"
	notFound targetCluster = "not-found"
)

func (c targetCluster) getClusterName() string {
	return string(c)
}

// getClusterIfApproved checks if the user can be approved and provisioned to any member cluster.
// If the user can be approved then the function returns true as the first returned value, the second value contains a cluster name the user should be provisioned to.
// If there is no suitable member cluster, then it returns notFound as the second returned value.
//
// If the user is approved manually then it tries to get member cluster with enough capacity if the target cluster is not already specified for UserSignup.
// If the user is not approved manually, then it loads HostOperatorConfig to check if automatic approval is enabled or not. If it is then it checks
// capacity thresholds and the actual use if there is any suitable member cluster. If it is not then it returns false as the first value and
// targetCluster unknown as the second value.
func getClusterIfApproved(cl client.Client, userSignup *toolchainv1alpha1.UserSignup, getMemberClusters cluster.GetMemberClustersFunc) (bool, targetCluster, error) {
	config, err := hostoperatorconfig.GetConfig(cl, userSignup.Namespace)
	if err != nil {
		return false, unknown, errors.Wrapf(err, "unable to read HostOperatorConfig resource")
	}

	if !states.Approved(userSignup) && !config.AutomaticApproval.Enabled {
		return false, unknown, nil
	}

	status := &toolchainv1alpha1.ToolchainStatus{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: crtCfg.ToolchainStatusName}, status); err != nil {
		return false, unknown, errors.Wrapf(err, "unable to read ToolchainStatus resource")
	}
	counts, err := counter.GetCounts()
	if err != nil {
		return false, unknown, errors.Wrapf(err, "unable to get the number of provisioned users")
	}

	clusterName := getOptimalTargetCluster(userSignup, getMemberClusters, hasNotReachedMaxNumberOfUsersThreshold(config, counts), hasEnoughResources(config, status))
	if clusterName == "" {
		return states.Approved(userSignup), notFound, nil
	}
	return true, targetCluster(clusterName), nil
}

func hasNotReachedMaxNumberOfUsersThreshold(config toolchainv1alpha1.HostOperatorConfigSpec, counts counter.Counts) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		if config.AutomaticApproval.MaxNumberOfUsers.Overall != 0 {
			if config.AutomaticApproval.MaxNumberOfUsers.Overall <= counts.MasterUserRecordCount {
				return false
			}
		}
		numberOfUserAccounts := counts.UserAccountsPerClusterCounts[cluster.Name]
		threshold := config.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster[cluster.Name]
		return threshold == 0 || numberOfUserAccounts < threshold
	}
}

func hasEnoughResources(config toolchainv1alpha1.HostOperatorConfigSpec, status *toolchainv1alpha1.ToolchainStatus) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		threshold, found := config.AutomaticApproval.ResourceCapacityThreshold.SpecificPerMemberCluster[cluster.Name]
		if !found {
			threshold = config.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold
		}
		if threshold == 0 {
			return true
		}
		for _, memberStatus := range status.Status.Members {
			if memberStatus.ClusterName == cluster.Name {
				return hasMemberEnoughResources(memberStatus, threshold)
			}
		}
		return false
	}
}

func hasMemberEnoughResources(memberStatus toolchainv1alpha1.Member, threshold int) bool {
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

func getOptimalTargetCluster(userSignup *toolchainv1alpha1.UserSignup, getMemberClusters cluster.GetMemberClustersFunc, conditions ...cluster.Condition) string {
	// If a target cluster hasn't been selected, select one from the members
	if userSignup.Spec.TargetCluster != "" {
		return userSignup.Spec.TargetCluster
	}
	// Automatic cluster selection based on cluster readiness
	members := getMemberClusters(append(conditions, cluster.Ready)...)

	if len(members) > 0 {
		return members[0].Name
	}
	return ""
}
