package usersignup

import (
	"testing"

	. "github.com/codeready-toolchain/host-operator/test"
	spacebindingtest "github.com/codeready-toolchain/host-operator/test/spacebinding"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
)

func TestNewSpaceBinding(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	nsTemplateTier := newNsTemplateTier("advanced", "dev", "stage", "extra")
	space := newSpace(userSignup, test.MemberClusterName, "smith", nsTemplateTier.Name)
	mur := newMasterUserRecord(userSignup, test.MemberClusterName, nsTemplateTier, "johny")

	// when
	actualSpaceBinding := newSpaceBinding(mur, space, userSignup.Name)

	// then
	expectedSpaceBinding := spacebindingtest.NewSpaceBinding("johny", "smith", "admin", userSignup.Name)
	assert.Equal(t, expectedSpaceBinding, actualSpaceBinding)
}
