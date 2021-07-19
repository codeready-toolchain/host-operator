package unapproved

import (
	"context"
	"fmt"
	"testing"

	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMapperReturnsOldest(t *testing.T) {
	// given
	pending := NewUserSignup(WithStateLabel("pending"))
	approved := NewUserSignup(WithStateLabel("approved"))
	deactivated := NewUserSignup(WithStateLabel("deactivated"))
	cl := test.NewFakeClient(t, pending, approved, deactivated)
	mapper := NewUserSignupMapper(cl)

	// when
	requests := mapper.MapToOldestUnapproved(NewToolchainStatus())

	// then
	require.Len(t, requests, 1)
	assert.Equal(t, requests[0].Name, pending.Name)
	assert.Equal(t, requests[0].Namespace, test.HostOperatorNs)
	approve(t, cl, pending)
}

func TestMapperReturnsEmptyRequestsWhenNoPendingIsFound(t *testing.T) {
	// given
	banned := NewUserSignup(WithStateLabel("banned"))
	approved := NewUserSignup(WithStateLabel("approved"))
	deactivated := NewUserSignup(WithStateLabel("deactivated"))
	cl := test.NewFakeClient(t, banned, approved, deactivated)
	mapper := NewUserSignupMapper(cl)

	// when
	requests := mapper.MapToOldestUnapproved(NewToolchainStatus())

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
	requests := mapper.MapToOldestUnapproved(NewToolchainStatus())

	// then
	assert.Empty(t, requests)
}
