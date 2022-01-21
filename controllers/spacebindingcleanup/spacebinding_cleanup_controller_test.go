package spacebindingcleanup

import (
	"context"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/test/space"
	sb "github.com/codeready-toolchain/host-operator/test/spacebinding"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestDeleteSpaceBinding(t *testing.T) {
	// given
	sbLaraRedhatAdmin := sb.NewSpaceBinding("lara", "redhat", "admin")
	sbJoeRedhatView := sb.NewSpaceBinding("joe", "redhat", "view")
	sbLaraIbmEdit := sb.NewSpaceBinding("lara", "ibm", "edit")

	redhatSpace := space.NewSpace("redhat")
	ibmSpace := space.NewSpace("ibm")

	laraMur := masteruserrecord.NewMasterUserRecord(t, "lara")
	joeMur := masteruserrecord.NewMasterUserRecord(t, "joe")

	t.Run("lara-admin SpaceBinding removed when redhat space is missing", func(t *testing.T) {

		reconciler, request, fakeClient := prepareReconciler(t, sbLaraRedhatAdmin, sbJoeRedhatView, sbLaraIbmEdit, laraMur, joeMur, ibmSpace)

		// when
		_, err := reconciler.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbLaraRedhatAdmin.Name, fakeClient).DoesNotExist()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbJoeRedhatView.Name, fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbLaraIbmEdit.Name, fakeClient).Exists()
	})

	t.Run("joe-view SpaceBinding removed when joe MUR space is missing", func(t *testing.T) {

		reconciler, request, fakeClient := prepareReconciler(t, sbJoeRedhatView, sbLaraRedhatAdmin, sbLaraIbmEdit, laraMur, ibmSpace, redhatSpace)

		// when
		_, err := reconciler.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbLaraRedhatAdmin.Name, fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbJoeRedhatView.Name, fakeClient).DoesNotExist()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbLaraIbmEdit.Name, fakeClient).Exists()
	})

	t.Run("no SpaceBinding removed when MUR and Space are present", func(t *testing.T) {

		reconciler, request, fakeClient := prepareReconciler(t, sbLaraRedhatAdmin, sbJoeRedhatView, sbLaraIbmEdit, laraMur, joeMur, ibmSpace, redhatSpace)

		// when
		_, err := reconciler.Reconcile(context.TODO(), request)

		// then
		require.NoError(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbLaraRedhatAdmin.Name, fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbJoeRedhatView.Name, fakeClient).Exists()
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbLaraIbmEdit.Name, fakeClient).Exists()
	})

	t.Run("fails while getting the bound resource", func(t *testing.T) {

		for _, boundResourceName := range []string{"lara", "redhat"} {
			reconciler, request, fakeClient := prepareReconciler(t, sbLaraRedhatAdmin, redhatSpace, laraMur)
			fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if key.Name == boundResourceName {
					return fmt.Errorf("some error")
				}
				return fakeClient.Client.Get(ctx, key, obj)
			}

			// when
			_, err := reconciler.Reconcile(context.TODO(), request)

			// then
			require.Error(t, err)
			sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbLaraRedhatAdmin.Name, fakeClient).Exists()
		}
	})

	t.Run("fails while deleting the SpaceBinding", func(t *testing.T) {

		reconciler, request, fakeClient := prepareReconciler(t, sbLaraRedhatAdmin, redhatSpace)
		fakeClient.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			return fmt.Errorf("some error")
		}

		// when
		_, err := reconciler.Reconcile(context.TODO(), request)

		// then
		require.Error(t, err)
		sb.AssertThatSpaceBinding(t, test.HostOperatorNs, sbLaraRedhatAdmin.Name, fakeClient).Exists()
	})
}

func prepareReconciler(t *testing.T, sb *v1alpha1.SpaceBinding, initObjects ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	cl := test.NewFakeClient(t, append(initObjects, sb)...)

	reconciler := &Reconciler{
		Namespace: test.HostOperatorNs,
		Scheme:    s,
		Client:    cl,
	}
	req := reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, sb.Name),
	}
	return reconciler, req, cl
}
