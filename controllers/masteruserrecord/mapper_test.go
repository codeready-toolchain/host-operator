package masteruserrecord

import (
	"context"
	"fmt"
	"testing"

	spacebindingtest "github.com/codeready-toolchain/host-operator/test/spacebinding"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestBannedUserToUserSignupMapper(t *testing.T) {
	//given
	spaceBinding := spacebindingtest.NewSpaceBinding("john", "john-space", "admin", "john-123")
	spaceBindingJane := spacebindingtest.NewSpaceBinding("jane", "john-space", "admin", "jane-123")
	noise := spacebindingtest.NewSpaceBinding("mr-noise", "mr-noise-space", "admin", "mr-noise")
	space := spacetest.NewSpace(test.HostOperatorNs, "john-space")

	t.Run("with one SpaceBinding", func(t *testing.T) {
		// given
		client := test.NewFakeClient(t, spaceBinding, noise)

		// when
		requests := MapSpaceToMasterUserRecord(client)(space)

		// then
		assert.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest("john"))
	})

	t.Run("with two SpaceBindings", func(t *testing.T) {
		// given
		client := test.NewFakeClient(t, spaceBinding, spaceBindingJane, noise)

		// when
		requests := MapSpaceToMasterUserRecord(client)(space)

		// then
		assert.Len(t, requests, 2)
		assert.Contains(t, requests, newRequest("john"))
		assert.Contains(t, requests, newRequest("jane"))
	})

	t.Run("without any SpaceBinding", func(t *testing.T) {
		// given
		client := test.NewFakeClient(t, noise)

		// when
		requests := MapSpaceToMasterUserRecord(client)(space)

		// then
		assert.Len(t, requests, 0)
	})

	t.Run("when list fails", func(t *testing.T) {
		// given
		client := test.NewFakeClient(t, noise)
		client.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			return fmt.Errorf("dummy error")
		}

		// when
		requests := MapSpaceToMasterUserRecord(client)(space)

		// then
		assert.Len(t, requests, 0)
	})
}

func newRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      name,
		},
	}
}
