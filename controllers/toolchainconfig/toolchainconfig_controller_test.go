package toolchainconfig_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
	// given
	defaultMemberConfig := testconfig.NewMemberOperatorConfig(testconfig.MemberStatus().RefreshPeriod("5s"))
	specificMemberConfig := testconfig.NewMemberOperatorConfig(testconfig.MemberStatus().RefreshPeriod("10s"))

	t.Run("success", func(t *testing.T) {

		t.Run("config not found", func(t *testing.T) {
			hostCl := test.NewFakeClient(t)
			member1 := NewMemberCluster(t, "member1", v1.ConditionTrue)
			member2 := NewMemberCluster(t, "member2", v1.ConditionTrue)
			members := NewGetMemberClusters(member1, member2)
			controller := newController(hostCl, members)

			// when
			res, err := controller.Reconcile(newRequest())

			// then
			require.Empty(t, res)
			require.NoError(t, err)
			actual, err := toolchainconfig.GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
			require.NoError(t, err)
			matchesDefaultConfig(t, actual)
			testconfig.AssertThatToolchainConfig(t, test.HostOperatorNs, hostCl).NotExists()

			// check member1 config
			_, err = getMemberConfig(member1)
			assert.Error(t, err)
			assert.True(t, errors.IsNotFound(err))

			// check member2 config
			_, err = getMemberConfig(member2)
			assert.Error(t, err)
			assert.True(t, errors.IsNotFound(err))

		})

		t.Run("config exists", func(t *testing.T) {
			config := newToolchainConfigWithReset(t,
				testconfig.AutomaticApproval().Enabled().MaxUsersNumber(123, testconfig.PerMemberCluster("member1", 321)),
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member1", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			member1 := NewMemberCluster(t, "member1", v1.ConditionTrue)
			member2 := NewMemberCluster(t, "member2", v1.ConditionTrue)
			members := NewGetMemberClusters(member1, member2)
			controller := newController(hostCl, members)

			// when
			res, err := controller.Reconcile(newRequest())

			// then
			require.Equal(t, toolchainconfig.DefaultReconcile, res)
			require.NoError(t, err)
			actual, err := toolchainconfig.GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
			require.NoError(t, err)
			assert.True(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			testconfig.AssertThatToolchainConfig(t, test.HostOperatorNs, hostCl).Exists().HasConditions(toolchainconfig.ToBeComplete()).HasNoSyncErrors()

			// check member1 config
			member1Cfg, err := getMemberConfig(member1)
			assert.NoError(t, err)
			assert.Equal(t, "10s", *member1Cfg.Spec.MemberStatus.RefreshPeriod)

			// check member2 config
			member2Cfg, err := getMemberConfig(member2)
			assert.NoError(t, err)
			assert.Equal(t, "5s", *member2Cfg.Spec.MemberStatus.RefreshPeriod)

			t.Run("cache updated with new version", func(t *testing.T) {
				// given
				err := hostCl.Get(context.TODO(), types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, config)
				require.NoError(t, err)
				threshold := 100
				config.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold = &threshold
				newRefreshPeriod := "20s"
				config.Spec.Members.Default.MemberStatus.RefreshPeriod = &newRefreshPeriod
				err = hostCl.Update(context.TODO(), config)
				require.NoError(t, err)

				// when
				res, err := controller.Reconcile(newRequest())

				// then
				require.Equal(t, toolchainconfig.DefaultReconcile, res)
				require.NoError(t, err)
				actual, err := toolchainconfig.GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
				require.NoError(t, err)
				assert.True(t, actual.AutomaticApproval().IsEnabled())
				assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
				assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
				assert.Equal(t, 100, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
				testconfig.AssertThatToolchainConfig(t, test.HostOperatorNs, hostCl).Exists().HasConditions(toolchainconfig.ToBeComplete()).HasNoSyncErrors()

				// check member1 config is unchanged
				member1Cfg, err := getMemberConfig(member1)
				assert.NoError(t, err)
				assert.Equal(t, "10s", *member1Cfg.Spec.MemberStatus.RefreshPeriod)

				// check member2 config is updated
				member2Cfg, err := getMemberConfig(member2)
				assert.NoError(t, err)
				assert.Equal(t, "20s", *member2Cfg.Spec.MemberStatus.RefreshPeriod)
			})

			t.Run("subsequent get fail - cache should be same", func(t *testing.T) {
				// given
				err := hostCl.Get(context.TODO(), types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, config)
				require.NoError(t, err)
				threshold := 100
				config.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold = &threshold
				err = hostCl.Update(context.TODO(), config)
				require.NoError(t, err)
				hostCl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					return fmt.Errorf("client error")
				}

				// when
				res, err := controller.Reconcile(newRequest())

				// then
				require.Equal(t, toolchainconfig.DefaultReconcile, res)
				require.EqualError(t, err, "client error")
				actual, err := toolchainconfig.GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
				require.NoError(t, err)
				assert.True(t, actual.AutomaticApproval().IsEnabled())
				assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
				assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
				assert.Equal(t, 100, actual.AutomaticApproval().ResourceCapacityThresholdDefault())

				// check member1 config is unchanged
				member1Cfg, err := getMemberConfig(member1)
				assert.NoError(t, err)
				assert.Equal(t, "10s", *member1Cfg.Spec.MemberStatus.RefreshPeriod)

				// check member2 config is unchanged
				member2Cfg, err := getMemberConfig(member2)
				assert.NoError(t, err)
				assert.Equal(t, "20s", *member2Cfg.Spec.MemberStatus.RefreshPeriod)
			})
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("initial get failed", func(t *testing.T) {
			// given
			config := newToolchainConfigWithReset(t,
				testconfig.AutomaticApproval().Enabled().MaxUsersNumber(123, testconfig.PerMemberCluster("member1", 321)),
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member1", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(hostCl, members)
			hostCl.MockGet = func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				return fmt.Errorf("client error")
			}

			// when
			res, err := controller.Reconcile(newRequest())

			// then
			require.Equal(t, toolchainconfig.DefaultReconcile, res)
			require.EqualError(t, err, "client error")
			actual, err := toolchainconfig.GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
			require.NoError(t, err)
			matchesDefaultConfig(t, actual)
		})

		t.Run("sync failed", func(t *testing.T) {
			// given
			config := newToolchainConfigWithReset(t, testconfig.AutomaticApproval().Enabled().MaxUsersNumber(123, testconfig.PerMemberCluster("member1", 321)), testconfig.Members().Default(defaultMemberConfig.Spec), config.Members().SpecificPerMemberCluster("missing-member", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(hostCl, members)

			// when
			res, err := controller.Reconcile(newRequest())

			// then
			require.NoError(t, err)
			require.Equal(t, toolchainconfig.DefaultReconcile, res)
			actual, err := toolchainconfig.GetConfig(test.NewFakeClient(t), test.HostOperatorNs)
			require.NoError(t, err)
			assert.True(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 80, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
			testconfig.AssertThatToolchainConfig(t, test.HostOperatorNs, hostCl).Exists().HasConditions(toolchainconfig.ToSyncFailure()).HasSyncErrors(map[string]string{"missing-member": "specific member configuration exists but no matching toolchaincluster was found"})
		})
	})
}

func newRequest() reconcile.Request {
	return reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, "config"),
	}
}

func newToolchainConfigWithReset(t *testing.T, options ...testconfig.ToolchainConfigOption) *toolchainv1alpha1.ToolchainConfig {
	t.Cleanup(toolchainconfig.Reset)
	return testconfig.NewToolchainConfig(options...)
}

func matchesDefaultConfig(t *testing.T, actual toolchainconfig.ToolchainConfig) {
	assert.False(t, actual.AutomaticApproval().IsEnabled())
	assert.Equal(t, 1000, actual.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
	assert.Equal(t, 80, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
	assert.Empty(t, actual.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	assert.Equal(t, 3, actual.Deactivation().DeactivatingNotificationInDays())
}

func newController(hostCl client.Client, members cluster.GetMemberClustersFunc) toolchainconfig.Reconciler {
	return toolchainconfig.Reconciler{
		Client:         hostCl,
		Log:            ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
		GetMembersFunc: members,
	}
}

func getMemberConfig(cluster *cluster.CachedToolchainCluster) (*toolchainv1alpha1.MemberOperatorConfig, error) {
	memberConfig := &toolchainv1alpha1.MemberOperatorConfig{}
	err := cluster.Client.Get(context.TODO(), test.NamespacedName(cluster.OperatorNamespace, "config"), memberConfig)
	return memberConfig, err
}
