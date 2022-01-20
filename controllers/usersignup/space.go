package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newSpace(config toolchainconfig.ToolchainConfig, userSignup *toolchainv1alpha1.UserSignup, targetCluster targetCluster, compliantUserName string) *toolchainv1alpha1.Space {
	labels := map[string]string{
		toolchainv1alpha1.SpaceCreatorLabelKey: compliantUserName,
	}

	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: userSignup.Namespace,
			Name:      compliantUserName,
			Labels:    labels,
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TargetCluster: targetCluster.getClusterName(),
			TierName:      config.Tiers().DefaultSpaceTier(),
		},
	}
	return space
}
