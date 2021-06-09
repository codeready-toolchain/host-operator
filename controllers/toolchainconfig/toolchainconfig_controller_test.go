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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
	// given
	defaultMemberConfig := test.NewMemberOperatorConfig(test.MemberStatus().RefreshPeriod("5s"))
	specificMemberConfig := test.NewMemberOperatorConfig(test.MemberStatus().RefreshPeriod("10s"))

	t.Run("success", func(t *testing.T) {

		t.Run("config not found", func(t *testing.T) {
			hostCl := test.NewFakeClient(t)
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(hostCl, members)

			// when
			res, err := controller.Reconcile(newRequest())

			// then
			require.Empty(t, res)
			require.NoError(t, err)
			actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
			require.NoError(t, err)
			matchesDefaultConfig(t, actual)
		})

		t.Run("config exists", func(t *testing.T) {
			config := newToolchainConfigWithReset(t, test.AutomaticApproval().Enabled().MaxUsersNumber(123, test.PerMemberCluster("member1", 321)), test.Members().Default(defaultMemberConfig.Spec), test.Members().SpecificPerMemberCluster("member1", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(hostCl, members)

			// when
			res, err := controller.Reconcile(newRequest())

			// then
			require.Equal(t, defaultReconcile, res)
			require.NoError(t, err)
			actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
			require.NoError(t, err)
			assert.True(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())

			t.Run("cache updated with new version", func(t *testing.T) {
				// given
				threshold := 100
				config.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold = &threshold
				err := hostCl.Update(context.TODO(), config)
				require.NoError(t, err)

				// when
				res, err := controller.Reconcile(newRequest())

				// then
				require.Equal(t, defaultReconcile, res)
				require.NoError(t, err)
				actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
				require.NoError(t, err)
				assert.True(t, actual.AutomaticApproval().IsEnabled())
				assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
				assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
				assert.Equal(t, 100, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
			})

			t.Run("subsequent get fail - cache should be same", func(t *testing.T) {
				// given
				threshold := 100
				config.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold = &threshold
				err := hostCl.Update(context.TODO(), config)
				require.NoError(t, err)
				hostCl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					return fmt.Errorf("client error")
				}

				// when
				res, err := controller.Reconcile(newRequest())

				// then
				require.Equal(t, defaultReconcile, res)
				require.EqualError(t, err, "client error")
				actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
				require.NoError(t, err)
				assert.True(t, actual.AutomaticApproval().IsEnabled())
				assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
				assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
				assert.Equal(t, 100, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
			})
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("initial get failed", func(t *testing.T) {
			// given
			config := newToolchainConfigWithReset(t, test.AutomaticApproval().Enabled().MaxUsersNumber(123, test.PerMemberCluster("member1", 321)), test.Members().Default(defaultMemberConfig.Spec), test.Members().SpecificPerMemberCluster("member1", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(hostCl, members)
			hostCl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				return fmt.Errorf("client error")
			}

			// when
			res, err := controller.Reconcile(newRequest())

			// then
			require.Equal(t, defaultReconcile, res)
			require.EqualError(t, err, "client error")
			actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
			require.NoError(t, err)
			matchesDefaultConfig(t, actual)
		})

		t.Run("sync failed", func(t *testing.T) {
			// given
			config := newToolchainConfigWithReset(t, test.AutomaticApproval().Enabled().MaxUsersNumber(123, test.PerMemberCluster("member1", 321)), test.Members().Default(defaultMemberConfig.Spec), test.Members().SpecificPerMemberCluster("missing-member", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(hostCl, members)

			// when
			res, err := controller.Reconcile(newRequest())

			// then
			require.NoError(t, err)
			require.Equal(t, defaultReconcile, res)
			actual, err := GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
			require.NoError(t, err)
			assert.True(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 80, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
		})
	})
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
