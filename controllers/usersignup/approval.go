package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/pkg/errors"
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
// If the user is not approved manually, then it loads ToolchainConfig to check if automatic approval is enabled or not. If it is then it checks
// capacity thresholds and the actual use if there is any suitable member cluster. If it is not then it returns false as the first value and
// targetCluster unknown as the second value.
func getClusterIfApproved(cl client.Client, userSignup *toolchainv1alpha1.UserSignup, getMemberClusters cluster.GetMemberClustersFunc) (bool, targetCluster, error) {
	config, err := toolchainconfig.GetToolchainConfig(cl)
	if err != nil {
		return false, unknown, errors.Wrapf(err, "unable to get ToolchainConfig")
	}

	if !states.Approved(userSignup) && !config.AutomaticApproval().IsEnabled() {
		return false, unknown, nil
	}

	// If a target cluster was specified, select it without any further checks, this is needed when users can only be provisioned to a specific member cluster
	if userSignup.Spec.TargetCluster != "" {
		return true, targetCluster(userSignup.Spec.TargetCluster), nil
	}

	// If the the UserSignup has a last target cluster annotation set it can be targeted to the same cluster, otherwise use the first one
	// The last cluster is used for returning users to ensure they can be provisioned back to the same cluster as they were previously using so they don't need to update URLs and kube contexts
	preferredCluster := userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey]

	clusterName, err := capacity.GetOptimalTargetCluster(preferredCluster, userSignup.Namespace, getMemberClusters, cl)
	if err != nil {
		return false, unknown, errors.Wrapf(err, "unable to get the optimal target cluster")
	}
	if clusterName == "" {
		return states.Approved(userSignup), notFound, nil
	}
	return true, targetCluster(clusterName), nil
}
