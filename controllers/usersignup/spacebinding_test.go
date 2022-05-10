package usersignup

import (
	"testing"

	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tiertest "github.com/codeready-toolchain/host-operator/test/nstemplatetier"
	testusertier "github.com/codeready-toolchain/host-operator/test/usertier"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSpaceBinding(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()
	nsTemplateTier := tiertest.NewNSTemplateTier("advanced", "dev", "stage", "extra")
	userTier := testusertier.NewUserTier("deactivate90", 90)
	space := newSpace(userSignup, test.MemberClusterName, "smith", nsTemplateTier.Name)
	mur := newMasterUserRecord(userSignup, test.MemberClusterName, userTier, "johny")

	// when
	actualSpaceBinding := newSpaceBinding(mur, space, userSignup.Name)

	// then
	assert.Equal(t, "johny", actualSpaceBinding.Spec.MasterUserRecord)
	assert.Equal(t, "smith", actualSpaceBinding.Spec.Space)
	assert.Equal(t, "admin", actualSpaceBinding.Spec.SpaceRole)

	require.NotNil(t, actualSpaceBinding.Labels)
	assert.Equal(t, userSignup.Name, actualSpaceBinding.Labels[toolchainv1alpha1.SpaceCreatorLabelKey])
	assert.Equal(t, "johny", actualSpaceBinding.Labels[toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey])
	assert.Equal(t, "smith", actualSpaceBinding.Labels[toolchainv1alpha1.SpaceBindingSpaceLabelKey])
}
