package usersignup

import (
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"
	"testing"

	spacetest "github.com/codeready-toolchain/host-operator/test/space"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
)

func TestNewSpace(t *testing.T) {
	// given
	userSignup := commonsignup.NewUserSignup()

	// when
	space := newSpace(userSignup, test.MemberClusterName, "johny", "advanced")

	// then
	expectedSpace := spacetest.NewSpace("johny",
		spacetest.WithTierName("advanced"),
		spacetest.WithSpecTargetCluster("member-cluster"),
		spacetest.WithCreatorLabel(userSignup.Name))
	assert.Equal(t, expectedSpace, space)
}
