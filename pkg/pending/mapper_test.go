package pending

import (
	"context"
	"fmt"
	"testing"

	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/host-operator/test/space"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMapperReturnsOldest(t *testing.T) {
	// given
	pendingSignup := commonsignup.NewUserSignup(commonsignup.WithStateLabel("pending"))
	approvedSignup := commonsignup.NewUserSignup(commonsignup.WithStateLabel("approved"))
	deactivatedSignup := commonsignup.NewUserSignup(commonsignup.WithStateLabel("deactivated"))

	pendingSpace := space.NewSpace("pending", space.WithStateLabel("pending"))
	clusterAssignedSpace := space.NewSpace("cluster-assigned", space.WithStateLabel("cluster-assigned"))

	cl := test.NewFakeClient(t, pendingSignup, approvedSignup, deactivatedSignup, pendingSpace, clusterAssignedSpace)

	t.Run("for UserSignup", func(t *testing.T) {
		// given
		mapper := NewUserSignupMapper(cl)

		// when
		requests := mapper.MapToOldestPending(NewToolchainStatus())

		// then
		require.Len(t, requests, 1)
		assert.Equal(t, requests[0].Name, pendingSignup.Name)
		assert.Equal(t, requests[0].Namespace, test.HostOperatorNs)
	})

	t.Run("for Spaces", func(t *testing.T) {
		// given
		mapper := NewSpaceMapper(cl)

		// when
		requests := mapper.MapToOldestPending(NewToolchainStatus())

		// then
		require.Len(t, requests, 1)
		assert.Equal(t, requests[0].Name, pendingSpace.Name)
		assert.Equal(t, requests[0].Namespace, test.HostOperatorNs)
	})
}

func TestMapperReturnsEmptyRequestsWhenNoPendingIsFound(t *testing.T) {
	// given
	banned := commonsignup.NewUserSignup(commonsignup.WithStateLabel("banned"))
	approved := commonsignup.NewUserSignup(commonsignup.WithStateLabel("approved"))
	deactivated := commonsignup.NewUserSignup(commonsignup.WithStateLabel("deactivated"))
	cl := test.NewFakeClient(t, banned, approved, deactivated)
	mapper := NewUserSignupMapper(cl)

	// when
	requests := mapper.MapToOldestPending(NewToolchainStatus())

	// then
	assert.Empty(t, requests)
}

func TestMapperReturnsEmptyRequestsWhenClientReturnsError(t *testing.T) {
	// given
	cl := test.NewFakeClient(t)
	cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		return fmt.Errorf("some error")
	}
	mapper := NewUserSignupMapper(cl)

	// when
	requests := mapper.MapToOldestPending(NewToolchainStatus())

	// then
	assert.Empty(t, requests)
}
