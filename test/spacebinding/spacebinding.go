package spacebinding

import (
	"fmt"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewSpaceBinding(mur, space, spaceRole, creator string) *v1alpha1.SpaceBinding {
	return &v1alpha1.SpaceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", mur, space),
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				v1alpha1.SpaceCreatorLabelKey:                 creator,
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
