package spaceprovisionerconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	hosttest "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test/assertions"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestSpaceProvisionerConfigReadinessTracking(t *testing.T) {
	blueprintSpc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs, ReferencingToolchainCluster("cluster1"), Enabled(true))

	t.Run("is ready when enabled, cluster present and enabled and enough capacity available", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80))

		r, req, cl := prepareReconcile(t, spc.DeepCopy(),
			readyToolchainCluster("cluster1"),
			hosttest.NewToolchainStatus(
				hosttest.WithMember("cluster1",
					hosttest.WithSpaceCount(3),
					hosttest.WithNodeRoleUsage("worker", 50),
				),
			),
		)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(Ready()), Has(ConsumedSpaceCount(3)), Has(ConsumedMemoryUsage(map[string]int{"worker": 50})))
	})

	t.Run("is not ready when disabled", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80), Enabled(false))

		r, req, cl := prepareReconcile(t, spc.DeepCopy(), readyToolchainCluster("cluster1"),
			hosttest.NewToolchainStatus(
				hosttest.WithMember("cluster1",
					hosttest.WithSpaceCount(3),
					hosttest.WithNodeRoleUsage("worker", 50),
				),
			),
		)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigDisabledReason)), Has(UnknownConsumedCapacity()))
	})

	t.Run("is not ready when cluster not present", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80))

		r, req, cl := prepareReconcile(t, spc.DeepCopy())

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason)), Has(UnknownConsumedCapacity()))
	})

	t.Run("is not ready when no cluster referenced", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80), ReferencingToolchainCluster(""))

		r, req, cl := prepareReconcile(t, spc.DeepCopy(),
			readyToolchainCluster("cluster1"),
			hosttest.NewToolchainStatus(
				hosttest.WithMember("cluster1",
					hosttest.WithSpaceCount(3),
					hosttest.WithNodeRoleUsage("worker", 50),
				),
			),
		)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason)), Has(UnknownConsumedCapacity()))
	})

	t.Run("is not ready with cluster not ready", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80))

		tc := readyToolchainCluster("cluster1")
		tc.Status.Conditions[0].Status = corev1.ConditionFalse

		r, req, cl := prepareReconcile(t, spc.DeepCopy(),
			tc,
			hosttest.NewToolchainStatus(
				hosttest.WithMember("cluster1",
					hosttest.WithSpaceCount(3),
					hosttest.WithNodeRoleUsage("worker", 50),
				),
			),
		)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotReadyReason)), Has(UnknownConsumedCapacity()))
	})

	t.Run("is not ready when space count is depleted", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80))

		r, req, cl := prepareReconcile(t, spc.DeepCopy(),
			readyToolchainCluster("cluster1"),
			hosttest.NewToolchainStatus(
				hosttest.WithMember("cluster1",
					hosttest.WithSpaceCount(5),
					hosttest.WithNodeRoleUsage("worker", 50),
				),
			),
		)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigInsufficientCapacityReason)), Has(ConsumedSpaceCount(5)), Has(ConsumedMemoryUsage(map[string]int{"worker": 50})))
	})

	t.Run("is not ready when memory is depleted in one", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80))

		r, req, cl := prepareReconcile(t, spc.DeepCopy(),
			readyToolchainCluster("cluster1"),
			hosttest.NewToolchainStatus(
				hosttest.WithMember("cluster1",
					hosttest.WithSpaceCount(3),
					hosttest.WithNodeRoleUsage("worker", 90),
					hosttest.WithNodeRoleUsage("master", 40),
				),
			),
		)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigInsufficientCapacityReason)), Has(ConsumedSpaceCount(3)), Has(ConsumedMemoryUsage(map[string]int{"worker": 90, "master": 40})))
	})

	t.Run("is not ready when memory is depleted in more", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80))

		r, req, cl := prepareReconcile(t, spc.DeepCopy(),
			readyToolchainCluster("cluster1"),
			hosttest.NewToolchainStatus(
				hosttest.WithMember("cluster1",
					hosttest.WithSpaceCount(3),
					hosttest.WithNodeRoleUsage("worker", 42),
					hosttest.WithNodeRoleUsage("master", 90),
					hosttest.WithNodeRoleUsage("magic", 90),
				),
			),
		)

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc,
			Is(NotReadyWithReason(toolchainv1alpha1.SpaceProvisionerConfigInsufficientCapacityReason)),
			Has(ConsumedSpaceCount(3)),
			Has(ConsumedMemoryUsage(map[string]int{"worker": 42, "master": 90, "magic": 90})))
	})

	t.Run("has ready unknown if consumed capacity not known", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80))

		r, req, cl := prepareReconcile(t, spc.DeepCopy(), readyToolchainCluster("cluster1"), hosttest.NewToolchainStatus())

		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Has(ReadyStatusAndReason(corev1.ConditionUnknown, toolchainv1alpha1.SpaceProvisionerConfigInsufficientCapacityReason)), Has(UnknownConsumedCapacity()))
	})

	t.Run("has ready unknown if memory capacity not known", func(t *testing.T) {
		// given
		spc := ModifySpaceProvisionerConfig(blueprintSpc.DeepCopy(), MaxNumberOfSpaces(5), MaxMemoryUtilizationPercent(80))

		r, req, cl := prepareReconcile(t, spc.DeepCopy(),
			readyToolchainCluster("cluster1"),
			hosttest.NewToolchainStatus(
				hosttest.WithMember("cluster1",
					hosttest.WithSpaceCount(3),
				),
			),
		)
		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Has(ReadyStatusAndReason(corev1.ConditionUnknown, toolchainv1alpha1.SpaceProvisionerConfigInsufficientCapacityReason)), Has(ConsumedSpaceCount(3)), Has(ConsumedMemoryUsage(nil)))
	})

	t.Run("zero means unlimited", func(t *testing.T) {
		// given
		spc := blueprintSpc.DeepCopy()

		r, req, cl := prepareReconcile(t, spc.DeepCopy(),
			readyToolchainCluster("cluster1"),
			hosttest.NewToolchainStatus(
				hosttest.WithMember("cluster1",
					hosttest.WithSpaceCount(3_000_000),
					hosttest.WithNodeRoleUsage("worker", 3000),
					hosttest.WithNodeRoleUsage("master", 800),
				),
			),
		)
		// when
		_, reconcileErr := r.Reconcile(context.TODO(), req)
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(spc), spc))

		// then
		assert.NoError(t, reconcileErr)
		AssertThat(t, spc, Is(Ready()), Has(ConsumedSpaceCount(3_000_000)), Has(ConsumedMemoryUsage(map[string]int{"master": 800, "worker": 3000})))
	})
}

func TestSpaceProvisionerConfigReEnqueing(t *testing.T) {
	spc := NewSpaceProvisionerConfig("spc", test.HostOperatorNs, ReferencingToolchainCluster("cluster1"), Enabled(true))

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
		require.Error(t, reconcileErr)
		AssertThat(t, spcInCluster, Is(ReadyStatusAndReason(corev1.ConditionFalse, toolchainv1alpha1.SpaceProvisionerConfigToolchainClusterNotFoundReason)))
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
		require.NoError(t, reconcileErr)
		assert.False(t, res.Requeue)
		assert.Empty(t, spc.Status.Conditions)
	})
	t.Run("doesn't re-enqueue when object being deleted", func(t *testing.T) {
		// given
		spc := spc.DeepCopy()
		spc.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
		controllerutil.AddFinalizer(spc, toolchainv1alpha1.FinalizerName)
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

func TestCollectConsumedCapacity(t *testing.T) {
	// given

	_, _, cl := prepareReconcile(t, nil,
		hosttest.NewToolchainStatus(
			hosttest.WithMember(
				"cluster-1",
				hosttest.WithSpaceCount(300),
				hosttest.WithNodeRoleUsage("master", 10),
				hosttest.WithNodeRoleUsage("worker", 40),
			),
			hosttest.WithMember(
				"cluster-2",
				hosttest.WithSpaceCount(1),
			),
		),
	)

	t.Run("returns the capacity when present", func(t *testing.T) {
		// when
		cc, err := collectConsumedCapacity(context.TODO(), cl, "cluster-1", test.HostOperatorNs)

		// then
		require.NoError(t, err)
		require.NotNil(t, cc)
		assert.Equal(t, 300, cc.SpaceCount)
		assert.Equal(t, map[string]int{"master": 10, "worker": 40}, cc.MemoryUsagePercentPerNodeRole)
	})

	t.Run("no memory usage is not an error", func(t *testing.T) {
		// when
		cc, err := collectConsumedCapacity(context.TODO(), cl, "cluster-2", test.HostOperatorNs)

		// then
		require.NoError(t, err)
		require.NotNil(t, cc)
		assert.Equal(t, 1, cc.SpaceCount)
		assert.Nil(t, cc.MemoryUsagePercentPerNodeRole)
	})

	t.Run("returns nil when no member status present", func(t *testing.T) {
		// when
		cc, err := collectConsumedCapacity(context.TODO(), cl, "unknown-cluster", test.HostOperatorNs)

		// then
		require.NoError(t, err)
		require.Nil(t, cc)
	})

	t.Run("returns error when no toolchain-status is found", func(t *testing.T) {
		// given
		toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
		require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKey{Name: "toolchain-status", Namespace: test.HostOperatorNs}, toolchainStatus))
		require.NoError(t, cl.Delete(context.TODO(), toolchainStatus))

		// when
		cc, err := collectConsumedCapacity(context.TODO(), cl, "unknown-cluster", test.HostOperatorNs)

		// then
		require.Error(t, err)
		require.Nil(t, cc)
	})

	t.Run("returns error on failure to get the toolchain status", func(t *testing.T) {
		// given
		cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			if key.Name == "toolchain-status" {
				return errors.New("intetionally failing")
			}
			return cl.Client.Get(ctx, key, obj, opts...)
		}

		// when
		cc, err := collectConsumedCapacity(context.TODO(), cl, "unknown-cluster", test.HostOperatorNs)

		// then
		require.Error(t, err)
		require.Nil(t, cc)
	})
}

func prepareReconcile(t *testing.T, spc *toolchainv1alpha1.SpaceProvisionerConfig, initObjs ...runtimeclient.Object) (*Reconciler, reconcile.Request, *test.FakeClient) {
	s := runtime.NewScheme()
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	objs := initObjs
	var name string
	var namespace string
	if spc != nil {
		objs = append(objs, spc)
		name = spc.Name
		namespace = spc.Namespace
	}
	fakeClient := test.NewFakeClient(t, objs...)

	r := &Reconciler{
		Client: fakeClient,
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
	}
	return r, req, fakeClient
}

func readyToolchainCluster(name string) *toolchainv1alpha1.ToolchainCluster { //nolint: unparam // it makes sense to have this param even if it always receives the same value
	return &toolchainv1alpha1.ToolchainCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
		Status: toolchainv1alpha1.ToolchainClusterStatus{
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:   toolchainv1alpha1.ConditionReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}
