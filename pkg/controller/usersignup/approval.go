package usersignup

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	crtCfg "github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getClusterIfApproved checks if the user can be approved and provisioned to any member cluster.
// If the user can be approved then the function returns true as the first returned value, the second value contains a cluster name the user should be provisioned to.
// If there is no suitable member cluster, then it returns empty string.
//
// If the user is approved manually then it tries to get member cluster with enough capacity if the target cluster is not already specified for UserSignup.
// If the user is not approved manually, then it loads HostOperatorConfig to check if automatic approval is enabled or not. If it is then it checks
// capacity thresholds and the actual use if there is any suitable member cluster.
func getClusterIfApproved(cl client.Client, crtConfig *crtCfg.Config, userSignup *toolchainv1alpha1.UserSignup, getMemberClusters cluster.GetMemberClustersFunc) (bool, string, error) {
	config := &toolchainv1alpha1.HostOperatorConfig{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: "config"}, config); err != nil {
		if apierrors.IsNotFound(err) {
			if userSignup.Spec.Approved {
				return true, getOptimalTargetCluster(userSignup, getMemberClusters), nil
			}
			return false, "", nil
		}
		return false, "", errors.Wrapf(err, "unable to read HostOperatorConfig resource")
	}
	if !userSignup.Spec.Approved && !config.Spec.AutomaticApproval.Enabled {
		return false, "", nil
	}

	status := &toolchainv1alpha1.ToolchainStatus{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Namespace: userSignup.Namespace, Name: crtConfig.GetToolchainStatusName()}, status); err != nil {
		return false, "", errors.Wrapf(err, "unable to read ToolchainStatus resource")
	}
	counts, err := counter.GetCounts()
	if err != nil {
		return false, "", errors.Wrapf(err, "unable to get the number of provisioned users")
	}

	targetCluster := getOptimalTargetCluster(userSignup, getMemberClusters, hasNotReachedMaxNumberOfUsersThreshold(config, counts), hasEnoughResources(config, status))
	if targetCluster == "" {
		return false, "", fmt.Errorf("no suitable member cluster found")
	}

	// approved manually
	if userSignup.Spec.Approved {
		return true, targetCluster, nil
	}

	if config.Spec.AutomaticApproval.MaxNumberOfUsers.Overall != 0 {

		if config.Spec.AutomaticApproval.MaxNumberOfUsers.Overall <= counts.MasterUserRecordCount {
			return false, "", nil
		}
	}

	if config.Spec.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold != 0 {
		if config.Spec.AutomaticApproval.MaxNumberOfUsers.Overall <= counts.MasterUserRecordCount {
			return false, "", nil
		}
	}
	return true, targetCluster, nil
}

func hasNotReachedMaxNumberOfUsersThreshold(config *toolchainv1alpha1.HostOperatorConfig, counts counter.Counts) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		numberOfUserAccounts := counts.UserAccountsPerClusterCounts[cluster.Name]
		threshold := config.Spec.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster[cluster.Name]
		return threshold == 0 || numberOfUserAccounts <= threshold
	}
}

func hasEnoughResources(config *toolchainv1alpha1.HostOperatorConfig, status *toolchainv1alpha1.ToolchainStatus) cluster.Condition {
	return func(cluster *cluster.CachedToolchainCluster) bool {
		threshold, found := config.Spec.AutomaticApproval.ResourceCapacityThreshold.SpecificPerMemberCluster[cluster.Name]
		if !found {
			threshold = config.Spec.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold
		}
		if threshold == 0 {
			return true
		}
		for _, memberStatus := range status.Status.Members {
			if memberStatus.ClusterName == cluster.Name {
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
		}
		return false
	}
}

func getOptimalTargetCluster(userSignup *toolchainv1alpha1.UserSignup, getMemberClusters cluster.GetMemberClustersFunc, conditions ...cluster.Condition) string {
	// If a target cluster hasn't been selected, select one from the members
	if userSignup.Spec.TargetCluster != "" {
		return userSignup.Spec.TargetCluster
	} else {
		// Automatic cluster selection based on cluster readiness
		members := getMemberClusters(append(conditions, cluster.Ready)...)

		if len(members) > 0 {
			return members[0].Name
		}
		return ""
	}
}
