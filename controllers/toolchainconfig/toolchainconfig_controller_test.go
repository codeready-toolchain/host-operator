package toolchainconfig

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileWhenToolchainConfigIsAvailable(t *testing.T) {
	// given
	config := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().MaxUsersNumber(123, testconfig.PerMemberCluster("member1", 321)))
	cl := NewFakeClient(t, config)
	controller := Reconciler{
		Client: cl,
		Log:    ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
	}

	// when
	_, err := controller.Reconcile(newRequest())

	// then
	require.NoError(t, err)
	actual, err := GetConfig(NewFakeClient(t), HostOperatorNs)
	require.NoError(t, err)
	assert.False(t, actual.AutomaticApproval().IsEnabled())
	assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

	t.Run("update with new version", func(t *testing.T) {
		// given
		threshold := 100
		config.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold = &threshold
		err := cl.Update(context.TODO(), config)
		require.NoError(t, err)

		// when
		_, err = controller.Reconcile(newRequest())

		// then
		require.NoError(t, err)
		actual, err := GetConfig(NewFakeClient(t), HostOperatorNs)
		require.NoError(t, err)
		assert.False(t, actual.AutomaticApproval().IsEnabled())
		assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		assert.Equal(t, 100, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
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
		Log:    ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
	}

	// when
	_, err := controller.Reconcile(newRequest())

	// then
	require.Error(t, err)
	actual, err := GetConfig(NewFakeClient(t), HostOperatorNs)
	require.NoError(t, err)
	matchesDefaultConfig(t, actual)
}

func TestReconcileWhenToolchainConfigIsNotPresent(t *testing.T) {
	// given
	controller := Reconciler{
		Client: NewFakeClient(t),
		Log:    ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
	}

	// when
	_, err := controller.Reconcile(newRequest())

	// then
	require.NoError(t, err)
	actual, err := GetConfig(NewFakeClient(t), HostOperatorNs)
	require.NoError(t, err)
	matchesDefaultConfig(t, actual)
}

func newRequest() reconcile.Request {
	return reconcile.Request{
		NamespacedName: NamespacedName(HostOperatorNs, "config"),
	}
}

func newToolchainConfigWithReset(t *testing.T, options ...testconfig.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	t.Cleanup(Reset)
	return testconfig.NewToolchainConfig(options...)
}

func matchesDefaultConfig(t *testing.T, actual ToolchainConfig) {
	assert.False(t, actual.AutomaticApproval().IsEnabled())
	assert.Equal(t, 1000, actual.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
	assert.Equal(t, 80, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
	assert.Empty(t, actual.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	assert.Equal(t, 3, actual.Deactivation().DeactivatingNotificationInDays())
}
