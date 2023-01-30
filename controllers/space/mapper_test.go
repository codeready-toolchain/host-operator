package space

import (
	"context"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/test/space"
	sb "github.com/codeready-toolchain/host-operator/test/spacebinding"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapToSubSpacesByParentObjectName(t *testing.T) {
	// given
	sbSpaceWithSubspaces := sb.NewSpaceBinding("jane", "parentSpace", "admin", "signupA")
	sbSpaceNoSubspaces := sb.NewSpaceBinding("joe", "single", "view", "signupB")
	// this space binding is not bound to any space
	sbOrphan := sb.NewSpaceBinding("orphan", "", "view", "signupC")

	// parent and sub-space are linked using the v1alpha1.ParentSpaceLabelKey label
	parentSpace := space.NewSpace("parentSpace")
	subSpace := space.NewSpace("subSpace", space.WithLabel(v1alpha1.ParentSpaceLabelKey, "parentSpace"))
	// following space has no sub-spaces
	singleSpace := space.NewSpace("single")

	cl := test.NewFakeClient(t, parentSpace, subSpace, singleSpace)

	t.Run("should return one Space when has no sub-spaces", func(t *testing.T) {
		// when
		requests := MapToSubSpacesByParentObjectName(cl)(sbSpaceNoSubspaces)

		// then
		require.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest(singleSpace.Name))
	})

	t.Run("should return two Spaces when there is a sub-space", func(t *testing.T) {
		// when
		requests := MapToSubSpacesByParentObjectName(cl)(sbSpaceWithSubspaces)

		// then
		require.Len(t, requests, 2)
		assert.Contains(t, requests, newRequest(parentSpace.Name))
		assert.Contains(t, requests, newRequest(subSpace.Name))
	})

	t.Run("should not return any Space request when there is no for the given SpaceBinding", func(t *testing.T) {
		// when
		requests := MapToSubSpacesByParentObjectName(cl)(sbOrphan)

		// then
		require.Empty(t, requests)
	})

	t.Run("should not return any Space requests when list fails", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t, parentSpace, subSpace, singleSpace)
		cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			return fmt.Errorf("some error")
		}

		// when
		requests := MapToSubSpacesByParentObjectName(cl)(sbSpaceNoSubspaces)

		// then
		require.Empty(t, requests)
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
