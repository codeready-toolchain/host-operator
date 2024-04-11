package spaceprovisionerconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test/assertions"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"
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
		spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs, ReferencingToolchainCluster("non-existent"))
		r, req, cl := prepareReconcile(t, spc)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason)))
	})

	t.Run("is valid when existing ToolchainCluster is referenced", func(t *testing.T) {
		// given
		spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs, ReferencingToolchainCluster("cluster1"))
		r, req, cl := prepareReconcile(t, spc, &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
		})

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(Ready()))
	})
	t.Run("is invalid when no ToolchainCluster is referenced", func(t *testing.T) {
		// given
		spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs)
		r, req, cl := prepareReconcile(t, spc)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason)))
	})
	t.Run("becomes valid when the referenced ToolchainCluster appears", func(t *testing.T) {
		// given
		spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs,
			ReferencingToolchainCluster("cluster1"),
			WithReadyConditionInvalid(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason))
		r, req, cl := prepareReconcile(t, spc, &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
		})

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(Ready()))
	})
	t.Run("becomes invalid when the referenced ToolchainCluster disappears", func(t *testing.T) {
		// given
		spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs,
			ReferencingToolchainCluster("cluster1"),
			WithReadyConditionValid())
		r, req, cl := prepareReconcile(t, spc)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason)))
	})
}

func TestSpaceProvisionerConfigReEnqueing(t *testing.T) {
	spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs, ReferencingToolchainCluster("cluster1"))

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
	t.Run("re-enqueues and reports error in status on failure to get ToolchainCluster", func(t *testing.T) {
		// given
		r, req, cl := prepareReconcile(t, spc.DeepCopy())
		getErr := errors.New("purposefully failing the get request")
		cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			if _, ok := obj.(*toolchainv1alpha1.ToolchainCluster); ok {
				return getErr
			}
			return cl.Client.Get(ctx, key, obj, opts...)
		}

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		spcInCluster := &toolchainv1alpha1.SpaceProvisionerConfig{}
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spcInCluster))

		// then
		assert.Error(t, reconcileErr)
		AssertThat(t, spcInCluster, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason)))
		assert.Len(t, spcInCluster.Status.Conditions, 1)
		assert.Equal(t, "failed to get the referenced ToolchainCluster: "+getErr.Error(), spcInCluster.Status.Conditions[0].Message)
	})
	t.Run("re-enqueues on failure to update the status", func(t *testing.T) {
		// given
		r, req, cl := prepareReconcile(t, spc.DeepCopy())

		expectedErr := errors.New("purposefully failing the get request")
		cl.MockStatusUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.SubResourceUpdateOption) error {
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
		res, reconcileErr := r.Reconcile(context.TODO(), req)

		// then
		assert.NoError(t, reconcileErr)
		assert.False(t, res.Requeue)
		assert.Empty(t, spc.Status.Conditions)
	})
	t.Run("doesn't re-enqueue when object being deleted", func(t *testing.T) {
		// given
		spc := spc.DeepCopy()
		spc.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
		r, req, cl := prepareReconcile(t, spc)

		// when
		res, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		assert.False(t, res.Requeue)
		assert.Empty(t, spc.Status.Conditions)
	})
	t.Run("doesn't re-enqueue when ToolchainCluster not found", func(t *testing.T) {
		// given
		spc := spc.DeepCopy()
		r, req, cl := prepareReconcile(t, spc)
		cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			if _, ok := obj.(*toolchainv1alpha1.ToolchainCluster); ok {
				return &kerrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}}
			}
			return cl.Client.Get(ctx, key, obj, opts...)
		}

		// when
		res, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		assert.False(t, res.Requeue)
		assert.NotEmpty(t, spc.Status.Conditions)
	})
}

func prepareReconcile(t *testing.T, spc *toolchainv1alpha1.SpaceProvisionerConfig, initObjs ...runtime.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	s := runtime.NewScheme()
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
