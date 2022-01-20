package spacebinding

import (
	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewSpaceBinding(name, mur, space, spaceRole string) *v1alpha1.SpaceBinding {
	return &v1alpha1.SpaceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				v1alpha1.SpaceBindingMasterUserRecordLabelKey: mur,
				v1alpha1.SpaceBindingSpaceLabelKey:            space,
			},
		},
		Spec: v1alpha1.SpaceBindingSpec{
			MasterUserRecord: mur,
			Space:            space,
			SpaceRole:        spaceRole,
		},
	}
}
