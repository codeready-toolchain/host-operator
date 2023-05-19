package space

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewSpace creates a space CR for a UserSignup object.
func NewSpace(userSignup *toolchainv1alpha1.UserSignup, targetClusterName string, compliantUserName, tier string) *toolchainv1alpha1.Space {
	labels := map[string]string{
		toolchainv1alpha1.SpaceCreatorLabelKey: userSignup.Name,
	}

	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: userSignup.Namespace,
			Name:      compliantUserName,
			Labels:    labels,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetCluster:      targetClusterName,
			TargetClusterRoles: []string{cluster.RoleLabel(cluster.Tenant)}, // by default usersignups should be provisioned to tenant clusters
			TierName:           tier,
		},
	}
	return space
}

// NewSubSpace creates a space CR for a SpaceRequest object.
func NewSubSpace(spaceRequest *toolchainv1alpha1.SpaceRequest, parentSpace *toolchainv1alpha1.Space) *toolchainv1alpha1.Space {
	labels := map[string]string{
		toolchainv1alpha1.SpaceRequestLabelKey:          spaceRequest.GetName(),
		toolchainv1alpha1.SpaceRequestNamespaceLabelKey: spaceRequest.GetNamespace(),
		toolchainv1alpha1.ParentSpaceLabelKey:           parentSpace.GetName(),
	}

	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: parentSpace.GetNamespace(),
			Name:      SubSpaceName(parentSpace),
			Labels:    labels,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetClusterRoles: spaceRequest.Spec.TargetClusterRoles,
			TierName:           spaceRequest.Spec.TierName,
			ParentSpace:        parentSpace.GetName(),
		},
	}

	// in case target cluster roles are not specified
	// let's set target cluster to be same of the parent space
	if len(spaceRequest.Spec.TargetClusterRoles) == 0 {
		space.Spec.TargetCluster = parentSpace.Spec.TargetCluster
	}

	return space
}

// SubSpaceName generates a name for a subSpace based on parentSpace names.
func SubSpaceName(parentSpace *toolchainv1alpha1.Space) string {
	return parentSpace.GetName() + "-subs"
}
