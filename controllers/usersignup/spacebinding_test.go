package usersignup

import (
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/codeready-toolchain/host-operator/pkg/space"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	"github.com/codeready-toolchain/toolchain-common/pkg/spacebinding"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSpaceBinding(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	nsTemplateTier := tiertest.NewNSTemplateTier("ourtier", "dev", "stage", "extra")
	space := NewSpace(userSignup, test.MemberClusterName, "smith", nsTemplateTier.Name)
	mur := newMasterUserRecord(userSignup, test.MemberClusterName, "deactivate90", "johny")

	// when
	actualSpaceBinding := spacebinding.NewSpaceBinding(mur, space, userSignup.Name)

	// then
	assert.Equal(t, "johny", actualSpaceBinding.Spec.MasterUserRecord)
	assert.Equal(t, "smith", actualSpaceBinding.Spec.Space)
	assert.Equal(t, "admin", actualSpaceBinding.Spec.SpaceRole)

	require.NotNil(t, actualSpaceBinding.Labels)
	assert.Equal(t, userSignup.Name, actualSpaceBinding.Labels[toolchainv1alpha1.SpaceCreatorLabelKey])
	assert.Equal(t, "johny", actualSpaceBinding.Labels[toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey])
	assert.Equal(t, "smith", actualSpaceBinding.Labels[toolchainv1alpha1.SpaceBindingSpaceLabelKey])
}
