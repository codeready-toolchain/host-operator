package hostoperatorconfig

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileWhenHostOperatorConfigIsAvailable(t *testing.T) {
	// given
	config := newHostOperatorConfigWithReset(t, AutomaticApproval().MaxUsersNumber(123, PerMemberCluster("member1", 321)))
	cl := NewFakeClient(t, config)
	controller := Reconciler{
		Client: cl,
		Log:    ctrl.Log.WithName("controllers").WithName("HostOperatorConfig"),
	}

	// when
	_, err := controller.Reconcile(newRequest())

	// then
	require.NoError(t, err)
	configSpec, err := GetConfig(NewFakeClient(t), HostOperatorNs)
	require.NoError(t, err)
	assert.Equal(t, config.Spec, configSpec)

	t.Run("update with new version", func(t *testing.T) {
		// given
		threshold := 100
		config.Spec.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold = &threshold
		err := cl.Update(context.TODO(), config)
		require.NoError(t, err)

		// when
		_, err = controller.Reconcile(newRequest())

		// then
		require.NoError(t, err)
		configSpec, err := GetConfig(NewFakeClient(t), HostOperatorNs)
		require.NoError(t, err)
		assert.Equal(t, config.Spec, configSpec)
	})
}

func TestReconcileWhenReturnsError(t *testing.T) {
	// given
	cl := NewFakeClient(t)
	cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
		return fmt.Errorf("some error")
	}
	controller := Reconciler{
		Client: cl,
		Log:    ctrl.Log.WithName("controllers").WithName("HostOperatorConfig"),
	}

	// when
	_, err := controller.Reconcile(newRequest())

	// then
	require.Error(t, err)
	configSpec, err := GetConfig(NewFakeClient(t), HostOperatorNs)
	require.NoError(t, err)
	assert.Equal(t, toolchainv1alpha1.HostConfig{}, configSpec)
}

func TestReconcileWhenHostOperatorConfigIsNotPresent(t *testing.T) {
	// given
	controller := Reconciler{
		Client: NewFakeClient(t),
		Log:    ctrl.Log.WithName("controllers").WithName("HostOperatorConfig"),
	}

	// when
	_, err := controller.Reconcile(newRequest())

	// then
	require.NoError(t, err)
	configSpec, err := GetConfig(NewFakeClient(t), HostOperatorNs)
	require.NoError(t, err)
	assert.Equal(t, toolchainv1alpha1.HostConfig{}, configSpec)
}

func newRequest() reconcile.Request {
	return reconcile.Request{
		NamespacedName: NamespacedName(HostOperatorNs, "config"),
	}
}
