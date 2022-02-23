package usersignup

import (
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const defaultSpaceRole = "admin"

func newSpaceBinding(mur *toolchainv1alpha1.MasterUserRecord, space *toolchainv1alpha1.Space, creator string) *toolchainv1alpha1.SpaceBinding {
	labels := map[string]string{
		toolchainv1alpha1.SpaceCreatorLabelKey:                 creator,
		toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey: mur.Name,
		toolchainv1alpha1.SpaceBindingSpaceLabelKey:            space.Name,
	}

	return &toolchainv1alpha1.SpaceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    mur.Namespace,
			GenerateName: spaceBindingName(mur.Name, space.Name) + "-",
			Labels:       labels,
		},
		Spec: toolchainv1alpha1.SpaceBindingSpec{
			MasterUserRecord: mur.Name,
			Space:            space.Name,
			SpaceRole:        defaultSpaceRole,
		},
	}
}

func spaceBindingName(murName, spaceName string) string {
	spaceBindingName := fmt.Sprintf("%s-%s", murName, spaceName)
	if len(spaceBindingName) > 50 {
		spaceBindingName = spaceBindingName[0:50]
	}
	return spaceBindingName
}
