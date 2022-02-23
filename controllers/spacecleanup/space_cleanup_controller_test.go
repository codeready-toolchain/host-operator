package spacecleanup_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/spacecleanup"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	spacetest "github.com/codeready-toolchain/host-operator/test/space"
	"github.com/codeready-toolchain/host-operator/test/spacebinding"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCleanupSpace(t *testing.T) {

	t.Run("without any SpaceBinding and created more than 30 seconds ago", func(t *testing.T) {
		// given
		space := spacetest.NewSpace("without-spacebinding", spacetest.WithCreationTimestamp(time.Now().Add(-31*time.Second)))
		r, req, cl := prepareReconcile(t, space)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
			DoesNotExist()
	})

	t.Run("without any SpaceBinding and created less than 30 seconds ago- Space shouldn't be deleted, just requeued", func(t *testing.T) {
		// given
		space := spacetest.NewSpace("without-spacebinding", spacetest.WithCreationTimestamp(time.Now().Add(-29*time.Second)))
		r, req, cl := prepareReconcile(t, space)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.LessOrEqual(t, res.RequeueAfter, time.Second)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
			Exists()
	})

	t.Run("with SpaceBinding - Space shouldn't be deleted", func(t *testing.T) {
		// given
		space := spacetest.NewSpace("with-spacebinding", spacetest.WithCreationTimestamp(time.Now().Add(-time.Minute)))
		spaceBinding := spacebinding.NewSpaceBinding("johny", space.Name, "admin")
		r, req, cl := prepareReconcile(t, space, spaceBinding)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
			Exists()
	})

	t.Run("when is already being deleted", func(t *testing.T) {
		// given
		space := spacetest.NewSpace("being-deleted", spacetest.WithDeletionTimestamp())
		r, req, cl := prepareReconcile(t, space)

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
		spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
			Exists()
	})

	t.Run("when the space is not there, then just skip it", func(t *testing.T) {
		// given
		space := spacetest.NewSpace("not-found") // will be disregarded, only included for call to prepareReconcile func
		r, req, _ := prepareReconcile(t, space)
		empty := test.NewFakeClient(t) // with space disregarded
		empty.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			return fmt.Errorf("shouldn't be called")
		}
		r.Client = empty

		// when
		res, err := r.Reconcile(context.TODO(), req)

		// then
		require.NoError(t, err)
		assert.False(t, res.Requeue)
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("when getting space fails", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("get-fails")
			r, req, cl := prepareReconcile(t, space)
			cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				return fmt.Errorf("some error")
			}

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.Error(t, err)
			assert.False(t, res.Requeue)
			cl.MockGet = nil
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				Exists()
		})

		t.Run("when list SpaceBindings fails", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("list-fails")
			r, req, cl := prepareReconcile(t, space)
			cl.MockList = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				return fmt.Errorf("some error")
			}

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.Error(t, err)
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				Exists()
		})

		t.Run("when delete Space fails", func(t *testing.T) {
			// given
			space := spacetest.NewSpace("delete-fails", spacetest.WithCreationTimestamp(time.Now().Add(-time.Minute)))
			r, req, cl := prepareReconcile(t, space)
			cl.MockDelete = func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
				return fmt.Errorf("some error")
			}

			// when
			res, err := r.Reconcile(context.TODO(), req)

			// then
			require.Error(t, err)
			assert.False(t, res.Requeue)
			spacetest.AssertThatSpace(t, test.HostOperatorNs, space.Name, cl).
				Exists()
		})
	})
}

func prepareReconcile(t *testing.T, space *toolchainv1alpha1.Space, initObjs ...runtime.Object) (*spacecleanup.Reconciler, reconcile.Request, *test.FakeClient) {
	require.NoError(t, os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs))
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	fakeClient := test.NewFakeClient(t, append(initObjs, space)...)

	r := &spacecleanup.Reconciler{
		Client:    fakeClient,
		Namespace: test.HostOperatorNs,
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      space.Name,
		},
	}
	return r, req, fakeClient
}
