package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newSpace(userSignup *toolchainv1alpha1.UserSignup, targetCluster targetCluster, compliantUserName, tier string) *toolchainv1alpha1.Space {
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
			TargetCluster: toolchainv1alpha1.TargetCluster{
				Name:  targetCluster.getClusterName(),
				Roles: []string{cluster.RoleLabel(cluster.Tenant)}, // by default usersignups should be provisioned to tenant clusters
			},
			TierName: tier,
		},
	}
	return space
}
