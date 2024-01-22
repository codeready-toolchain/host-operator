package spaceprovisionerconfig

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestSpaceProvisionerConfigValidation(t *testing.T) {
	t.Run("is not valid when non-existing ToolchainCluster is referenced", func(t *testing.T) {
		// given
		spc := &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "non-existent",
			},
		}
		r, req, cl := prepareReconcile(t, spc)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		getErr := cl.Get(context.TODO(), runtimeclient.ObjectKey{Name: "spc", Namespace: test.HostOperatorNs}, spc)

		// then
		require.NoError(t, reconcileErr)
		require.NoError(t, getErr)
		assert.True(t, condition.IsFalse(spc.Status.Conditions, toolchainv1alpha1.ConditionReady))
	})

	t.Run("is valid when existing ToolchainCluster is referenced", func(t *testing.T) {
		// given
		spc := &toolchainv1alpha1.SpaceProvisionerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "spc",
				Namespace: test.HostOperatorNs,
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "cluster1",
			},
		}
		r, req, cl := prepareReconcile(t, spc, &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
		})

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		getErr := cl.Get(context.TODO(), runtimeclient.ObjectKey{Name: "spc", Namespace: test.HostOperatorNs}, spc)

		// then
		require.NoError(t, reconcileErr)
		require.NoError(t, getErr)
		assert.True(t, condition.IsTrue(spc.Status.Conditions, toolchainv1alpha1.ConditionReady))
	})
}

func TestSpaceProvisionerConfigReEnqueing(t *testing.T) {
	spc := &toolchainv1alpha1.SpaceProvisionerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "spc",
			Namespace: test.HostOperatorNs,
		},
		Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
			ToolchainCluster: "cluster1",
		},
	}

	t.Run("re-enqueues on failure to GET", func(t *testing.T) {
		// given
		r, req, cl := prepareReconcile(t, spc.DeepCopy())

		expectedErr := errors.New("purposefully failing the get request")
		cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return expectedErr
		}

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)

		// then
		assert.ErrorIs(t, reconcileErr, expectedErr)
	})
	t.Run("re-enqueues on failure to update the status", func(t *testing.T) {
		// given
		r, req, cl := prepareReconcile(t, spc.DeepCopy())

		expectedErr := errors.New("purposefully failing the get request")
		cl.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			return expectedErr
		}

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)

		// then
		assert.ErrorIs(t, reconcileErr, expectedErr)
	})
	t.Run("doesn't re-enqueue when object not found", func(t *testing.T) {
		// given
		r, req, cl := prepareReconcile(t, spc.DeepCopy())

		cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return &kerrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}}
		}

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)

		// then
		assert.NoError(t, reconcileErr)
	})
	t.Run("doesn't re-enqueue when object being deleted", func(t *testing.T) {
		// given
		spc := spc.DeepCopy()
		spc.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
		r, req, _ := prepareReconcile(t, spc)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)

		// then
		assert.NoError(t, reconcileErr)
		assert.Empty(t, spc.Status.Conditions)
	})
}

func prepareReconcile(t *testing.T, spc *toolchainv1alpha1.SpaceProvisionerConfig, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	require.NoError(t, os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs))
	s := runtime.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	fakeClient := test.NewFakeClient(t, append(initObjs, spc)...)

	r := &Reconciler{
		Client: fakeClient,
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      spc.Name,
		},
	}
	return r, req, fakeClient
}
