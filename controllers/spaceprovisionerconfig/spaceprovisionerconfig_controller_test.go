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
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestSpaceProvisionerConfigValidation(t *testing.T) {
	t.Run("is not ready when non-existing ToolchainCluster is referenced", func(t *testing.T) {
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

	t.Run("is ready when existing ready ToolchainCluster is referenced", func(t *testing.T) {
		// given
		spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs, ReferencingToolchainCluster("cluster1"))
		r, req, cl := prepareReconcile(t, spc, &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.ToolchainClusterCondition{
					{
						Type:   toolchainv1alpha1.ToolchainClusterReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		})

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(Ready()))

		t.Run("and becomes not ready when ToolchainCluster becomes not ready", func(t *testing.T) {
			// given
			tc := &toolchainv1alpha1.ToolchainCluster{}
			require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKey{Name: "cluster1", Namespace: test.HostOperatorNs}, tc))
			tc.Status.Conditions = []toolchainv1alpha1.ToolchainClusterCondition{
				{
					Type:   toolchainv1alpha1.ToolchainClusterReady,
					Status: v1.ConditionFalse,
				},
			}
			require.NoError(t, cl.Update(context.TODO(), tc))

			// when
			_, reconcileErr := r.Reconcile(context.TODO(), req)
			require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

			// then
			assert.NoError(t, reconcileErr)
			AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotReadyReason)))
		})
	})

	t.Run("is not ready when no ToolchainCluster is referenced", func(t *testing.T) {
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

	t.Run("is not ready when existing not-ready ToolchainCluster is referenced", func(t *testing.T) {
		// given
		spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs, ReferencingToolchainCluster("cluster1"))
		r, req, cl := prepareReconcile(t, spc, &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.ToolchainClusterCondition{
					{
						Type:   toolchainv1alpha1.ToolchainClusterReady,
						Status: v1.ConditionFalse,
					},
				},
			},
		})

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotReadyReason)))

		t.Run("and becomes ready when the referenced ToolchainCluster becomes ready", func(t *testing.T) {
			// given
			tc := &toolchainv1alpha1.ToolchainCluster{}
			require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKey{Name: "cluster1", Namespace: test.HostOperatorNs}, tc))

			tc.Status.Conditions = []toolchainv1alpha1.ToolchainClusterCondition{
				{
					Type:   toolchainv1alpha1.ToolchainClusterReady,
					Status: v1.ConditionTrue,
				},
			}
			require.NoError(t, cl.Update(context.TODO(), tc))

			// when
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

			// then
			require.NoError(t, reconcileErr)
			AssertThat(t, spc, Is(Ready()))
		})
	})

	// note that this is checking we "jumping 2 steps" from toolchain cluster not being present at all to be present and ready
	t.Run("becomes ready when the referenced ToolchainCluster appears and is ready", func(t *testing.T) {
		// given
		spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs,
			ReferencingToolchainCluster("cluster1"),
			WithReadyConditionInvalid(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason))
		r, req, cl := prepareReconcile(t, spc, &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: test.HostOperatorNs,
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.ToolchainClusterCondition{
					{
						Type:   toolchainv1alpha1.ToolchainClusterReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		})

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(Ready()))
	})

	// "jumping 2 steps" from having a ready toolchain cluster to not having 1 at all
	t.Run("becomes not ready when the referenced ToolchainCluster disappears", func(t *testing.T) {
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

	// this is a variant of becoming not ready when the TC is not ready, but this time the TC loses the ready condition altogether.
	t.Run("becomes not ready when the referenced ToolchainCluster no longer has ready condition", func(t *testing.T) {
		// given
		spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs,
			ReferencingToolchainCluster("cluster1"),
			WithReadyConditionValid())
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
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotReadyReason)))
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
