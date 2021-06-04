package toolchainconfig

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/codeready-toolchain/host-operator/test"

	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileWhenToolchainConfigIsAvailable(t *testing.T) {
	// given
	config := newToolchainConfigWithReset(t, test.AutomaticApproval().MaxUsersNumber(123, test.PerMemberCluster("member1", 321)))

	t.Run("no members", func(t *testing.T) {
		hostCl := test.NewFakeClient(t, config)
		members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
		controller := newController(hostCl, members)

		// when
		_, err := controller.Reconcile(newRequest())

		// then
		require.NoError(t, err)
		actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
		require.NoError(t, err)
		assert.False(t, actual.AutomaticApproval().IsEnabled())
		assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

		t.Run("update with new version", func(t *testing.T) {
			// given
			threshold := 100
			config.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold = &threshold
			err := hostCl.Update(context.TODO(), config)
			require.NoError(t, err)

			// when
			_, err = controller.Reconcile(newRequest())

			// then
			require.NoError(t, err)
			actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
			require.NoError(t, err)
			assert.False(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 100, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
		})
	})
}

func TestReconcileWhenGetReturnsError(t *testing.T) {
	// given
	hostCl := test.NewFakeClient(t)
	hostCl.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
		return fmt.Errorf("some error")
	}
	members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
	controller := newController(hostCl, members)

	// when
	_, err := controller.Reconcile(newRequest())

	// then
	require.Error(t, err)
	actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
	require.NoError(t, err)
	matchesDefaultConfig(t, actual)
}

func TestReconcileWhenToolchainConfigIsNotPresent(t *testing.T) {
	// given
	controller := Reconciler{
		Client: test.NewFakeClient(t),
		Log:    ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
	}

	// when
	_, err := controller.Reconcile(newRequest())

	// then
	require.NoError(t, err)
	actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
	require.NoError(t, err)
	matchesDefaultConfig(t, actual)
}

func newRequest() reconcile.Request {
	return reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, "config"),
	}
}

func newToolchainConfigWithReset(t *testing.T, options ...test.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	t.Cleanup(Reset)
	return test.NewToolchainConfig(options...)
}

func matchesDefaultConfig(t *testing.T, actual ToolchainConfig) {
	assert.False(t, actual.AutomaticApproval().IsEnabled())
	assert.Equal(t, 1000, actual.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
	assert.Equal(t, 80, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
	assert.Empty(t, actual.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	assert.Equal(t, 3, actual.Deactivation().DeactivatingNotificationInDays())
}

func newController(hostCl client.Client, members cluster.GetMemberClustersFunc) Reconciler {
	return Reconciler{
		Client:         hostCl,
		Log:            ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
		GetMembersFunc: members,
	}
}
