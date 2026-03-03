package pending

import (
	"context"
	"fmt"
	"testing"

	toolchainstatustest "github.com/codeready-toolchain/host-operator/test/toolchainstatus"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMapperReturnsOldest(t *testing.T) {
	// given
	ctx := context.TODO()
	pendingSignup := commonsignup.NewUserSignup(commonsignup.WithStateLabel("pending"))
	approvedSignup := commonsignup.NewUserSignup(commonsignup.WithStateLabel("approved"))
	deactivatedSignup := commonsignup.NewUserSignup(commonsignup.WithStateLabel("deactivated"))

	pendingSpace := spacetest.NewSpace(test.HostOperatorNs, "pending", spacetest.WithStateLabel("pending"))
	clusterAssignedSpace := spacetest.NewSpace(test.HostOperatorNs, "cluster-assigned", spacetest.WithStateLabel("cluster-assigned"))

	cl := test.NewFakeClient(t, pendingSignup, approvedSignup, deactivatedSignup, pendingSpace, clusterAssignedSpace)

	t.Run("for UserSignup", func(t *testing.T) {
		// given
		mapper := NewUserSignupMapper(cl)

		// when
		requests := mapper.MapToOldestPending(ctx, toolchainstatustest.NewToolchainStatus())

		// then
		require.Len(t, requests, 1)
		assert.Equal(t, requests[0].Name, pendingSignup.Name)
		assert.Equal(t, test.HostOperatorNs, requests[0].Namespace)
	})

	t.Run("for Spaces", func(t *testing.T) {
		// given
		mapper := NewSpaceMapper(cl)

		// when
		requests := mapper.MapToOldestPending(ctx, toolchainstatustest.NewToolchainStatus())

		// then
		require.Len(t, requests, 1)
		assert.Equal(t, requests[0].Name, pendingSpace.Name)
		assert.Equal(t, test.HostOperatorNs, requests[0].Namespace)
	})
}

func TestMapperReturnsEmptyRequestsWhenNoPendingIsFound(t *testing.T) {
	// given
	ctx := context.TODO()
	banned := commonsignup.NewUserSignup(commonsignup.WithStateLabel("banned"))
	approved := commonsignup.NewUserSignup(commonsignup.WithStateLabel("approved"))
	deactivated := commonsignup.NewUserSignup(commonsignup.WithStateLabel("deactivated"))
	cl := test.NewFakeClient(t, banned, approved, deactivated)
	mapper := NewUserSignupMapper(cl)

	// when
	requests := mapper.MapToOldestPending(ctx, toolchainstatustest.NewToolchainStatus())

	// then
	assert.Empty(t, requests)
}

func TestMapperReturnsEmptyRequestsWhenClientReturnsError(t *testing.T) {
	// given
	ctx := context.TODO()
	cl := test.NewFakeClient(t)
	cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
		return fmt.Errorf("some error")
	}
	mapper := NewUserSignupMapper(cl)

	// when
	requests := mapper.MapToOldestPending(ctx, toolchainstatustest.NewToolchainStatus())

	// then
	assert.Empty(t, requests)
}
