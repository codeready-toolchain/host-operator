package space

import (
	"context"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	sb "github.com/codeready-toolchain/host-operator/test/spacebinding"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/space"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapToSubSpacesByParentObjectName(t *testing.T) {
	// given
	sbSpaceWithSubspaces := sb.NewSpaceBinding("jane", "parentSpace", "admin", "signupA")
	sbSpaceNoSubspaces := sb.NewSpaceBinding("joe", "single", "view", "signupB")
	// this space binding is not bound to any space
	sbOrphan := sb.NewSpaceBinding("orphan", "", "view", "signupC")

	// parent and sub-space are linked using the v1alpha1.ParentSpaceLabelKey label
	parentSpace := space.NewSpace(test.HostOperatorNs, "parentSpace")
	subSpace := space.NewSpace(test.HostOperatorNs, "subSpace", space.WithLabel(v1alpha1.ParentSpaceLabelKey, "parentSpace"))
	subSpace2 := space.NewSpace(test.HostOperatorNs, "subSpace2", space.WithLabel(v1alpha1.ParentSpaceLabelKey, "parentSpace"))
	// following are sub-subSpaces
	subSubSpace := space.NewSpace(test.HostOperatorNs, "subSubSpace", space.WithLabel(v1alpha1.ParentSpaceLabelKey, "subSpace"))
	subSubSpace2 := space.NewSpace(test.HostOperatorNs, "subSubSpace2", space.WithLabel(v1alpha1.ParentSpaceLabelKey, "subSpace"))
	// following space has no sub-spaces
	singleSpace := space.NewSpace(test.HostOperatorNs, "single")

	cl := test.NewFakeClient(t, parentSpace, subSpace, singleSpace, subSubSpace)

	t.Run("should return one Space when has no sub-spaces", func(t *testing.T) {
		// when
		requests := MapSpaceBindingToParentAndSubSpaces(cl)(sbSpaceNoSubspaces)

		// then
		require.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest(singleSpace.Name))
	})

	t.Run("should return 3 Spaces when there is a sub-subSpace", func(t *testing.T) {
		// when
		requests := MapSpaceBindingToParentAndSubSpaces(cl)(sbSpaceWithSubspaces)

		// then
		require.Len(t, requests, 3)
		assert.Contains(t, requests, newRequest(parentSpace.Name))
		assert.Contains(t, requests, newRequest(subSpace.Name))
	})

	t.Run("should not return any Space request when there is no for the given SpaceBinding", func(t *testing.T) {
		// when
		requests := MapSpaceBindingToParentAndSubSpaces(cl)(sbOrphan)

		// then
		require.Empty(t, requests)
	})

	t.Run("should not return any Space requests when list fails", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t, parentSpace, subSpace, singleSpace)
		cl.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			return fmt.Errorf("some error")
		}

		// when
		requests := MapSpaceBindingToParentAndSubSpaces(cl)(sbSpaceNoSubspaces)

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
