package usersignup

import (
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

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
		spacetest.WithSpecTargetClusterName("member-cluster"),
		spacetest.WithSpecTargetClusterRoles([]string{cluster.RoleLabel(cluster.Tenant)}),
		spacetest.WithCreatorLabel(userSignup.Name))
	assert.Equal(t, expectedSpace, space)
}
