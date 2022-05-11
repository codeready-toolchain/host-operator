package usertier

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewUserTier(tierName string, deactivationTimeoutDays int) *toolchainv1alpha1.UserTier {
	return &toolchainv1alpha1.UserTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      tierName,
		},
		Spec: toolchainv1alpha1.UserTierSpec{
			DeactivationTimeoutDays: deactivationTimeoutDays,
		},
	}
}
