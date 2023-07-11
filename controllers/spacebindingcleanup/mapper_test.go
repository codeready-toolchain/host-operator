package spacebindingcleanup

import (
	"context"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	sb "github.com/codeready-toolchain/host-operator/test/spacebinding"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMapToSpaceBindingByBoundObject(t *testing.T) {
	// given
	sbLaraCompAdmin := sb.NewSpaceBinding("lara", "comp", "admin", "signupA")
	sbJoeCompView := sb.NewSpaceBinding("joe", "comp", "view", "signupB")
	sbLaraOtherEdit := sb.NewSpaceBinding("lara", "other", "edit", "signupC")

	compSpace := spacetest.NewSpace(test.HostOperatorNs, "comp")
	orphanSpace := spacetest.NewSpace(test.HostOperatorNs, "orphan")

	laraMur := masteruserrecord.NewMasterUserRecord(t, "lara")
	joeMur := masteruserrecord.NewMasterUserRecord(t, "joe")
	orphanMur := masteruserrecord.NewMasterUserRecord(t, "orphan")

	cl := test.NewFakeClient(t, sbLaraCompAdmin, sbJoeCompView, sbLaraOtherEdit)

	t.Run("should return two SpaceBinding requests for comp space, when mapping from space", func(t *testing.T) {
		// when
		requests := MapToSpaceBindingByBoundObjectName(cl, v1alpha1.SpaceBindingSpaceLabelKey)(compSpace)

		// then
		require.Len(t, requests, 2)
		assert.Contains(t, requests, newRequest(sbLaraCompAdmin.Name))
		assert.Contains(t, requests, newRequest(sbJoeCompView.Name))
	})

	t.Run("should return two SpaceBindings for lara MUR, when mapping from mur", func(t *testing.T) {
		// when
		requests := MapToSpaceBindingByBoundObjectName(cl, v1alpha1.SpaceBindingMasterUserRecordLabelKey)(laraMur)

		// then
		require.Len(t, requests, 2)
		assert.Contains(t, requests, newRequest(sbLaraCompAdmin.Name))
		assert.Contains(t, requests, newRequest(sbLaraOtherEdit.Name))
	})

	t.Run("should return one SpaceBinding request for joe MUR, when mapping from mur", func(t *testing.T) {
		// when
		requests := MapToSpaceBindingByBoundObjectName(cl, v1alpha1.SpaceBindingMasterUserRecordLabelKey)(joeMur)

		// then
		require.Len(t, requests, 1)
		assert.Contains(t, requests, newRequest(sbJoeCompView.Name))
	})

	t.Run("should not return any SpaceBinding request when there is no for the given space", func(t *testing.T) {
		// when
		requests := MapToSpaceBindingByBoundObjectName(cl, v1alpha1.SpaceBindingSpaceLabelKey)(orphanSpace)

		// then
		require.Empty(t, requests)
	})

	t.Run("should not return any SpaceBinding request when there is no for the given MUR", func(t *testing.T) {
		// when
		requests := MapToSpaceBindingByBoundObjectName(cl, v1alpha1.SpaceBindingMasterUserRecordLabelKey)(orphanMur)

		// then
		require.Empty(t, requests)
	})

	t.Run("should not return any SpaceBinding requests when list fails", func(t *testing.T) {
		// given
		cl := test.NewFakeClient(t, sbLaraCompAdmin, sbJoeCompView, sbLaraOtherEdit)
		cl.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			return fmt.Errorf("some error")
		}

		// when
		requests := MapToSpaceBindingByBoundObjectName(cl, v1alpha1.SpaceBindingMasterUserRecordLabelKey)(orphanMur)

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
