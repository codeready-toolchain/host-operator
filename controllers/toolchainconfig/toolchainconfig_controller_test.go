package toolchainconfig_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/registrationservice"
	. "github.com/codeready-toolchain/host-operator/test"

	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
	// given
	defaultMemberConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("5s"))
	specificMemberConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("10s"))

	t.Run("success", func(t *testing.T) {

		t.Run("config not found", func(t *testing.T) {
			hostCl := test.NewFakeClient(t)
			member1 := NewMemberCluster(t, "member1", v1.ConditionTrue)
			member2 := NewMemberCluster(t, "member2", v1.ConditionTrue)
			members := NewGetMemberClusters(member1, member2)
			controller := newController(t, hostCl, members)

			// when
			res, err := controller.Reconcile(context.TODO(), newRequest())

			// then
			require.Empty(t, res)
			require.NoError(t, err)
			actual, err := toolchainconfig.GetToolchainConfig(hostCl)
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
			config := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.AutomaticApproval().Enabled(true).MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)),
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member1", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			member1 := NewMemberCluster(t, "member1", v1.ConditionTrue)
			member2 := NewMemberCluster(t, "member2", v1.ConditionTrue)
			members := NewGetMemberClusters(member1, member2)
			controller := newController(t, hostCl, members)

			// when
			res, err := controller.Reconcile(context.TODO(), newRequest())

			// then
			require.NoError(t, err)
			require.Equal(t, toolchainconfig.DefaultReconcile, res)
			actual, err := toolchainconfig.GetToolchainConfig(hostCl)
			require.NoError(t, err)
			assert.True(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			testconfig.AssertThatToolchainConfig(t, test.HostOperatorNs, hostCl).
				Exists().
				HasConditions(
					toolchainconfig.ToSyncComplete(),
					toolchainconfig.ToRegServiceDeploying("updated resources: [ServiceAccount: registration-service Role: registration-service RoleBinding: registration-service Deployment: registration-service Service: registration-service Route: registration-service]")).
				HasNoSyncErrors()

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
				res, err := controller.Reconcile(context.TODO(), newRequest())

				// then
				require.NoError(t, err)
				require.Equal(t, toolchainconfig.DefaultReconcile, res)
				actual, err := toolchainconfig.GetToolchainConfig(hostCl)
				require.NoError(t, err)
				assert.True(t, actual.AutomaticApproval().IsEnabled())
				assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
				assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
				assert.Equal(t, 100, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
				testconfig.AssertThatToolchainConfig(t, test.HostOperatorNs, hostCl).
					Exists().
					HasConditions(
						toolchainconfig.ToSyncComplete(),
						toolchainconfig.ToRegServiceDeployComplete()).
					HasNoSyncErrors()

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
				hostCl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					return fmt.Errorf("client error")
				}

				// when
				res, err := controller.Reconcile(context.TODO(), newRequest())

				// then
				require.EqualError(t, err, "client error")
				require.Equal(t, toolchainconfig.DefaultReconcile, res)
				actual, err := toolchainconfig.GetToolchainConfig(hostCl)
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

		t.Run("error getting the toolchainconfig resource", func(t *testing.T) {
			// given
			config := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.AutomaticApproval().Enabled(true).MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)),
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member1", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(t, hostCl, members)
			hostCl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				_, ok := obj.(*toolchainv1alpha1.ToolchainConfig)
				if ok {
					return fmt.Errorf("client error")
				}
				return hostCl.Client.Get(ctx, key, obj)
			}

			// when
			res, err := controller.Reconcile(context.TODO(), newRequest())

			// then
			require.EqualError(t, err, "client error")
			require.Equal(t, toolchainconfig.DefaultReconcile, res)
			actual, err := toolchainconfig.GetToolchainConfig(hostCl)
			require.EqualError(t, err, "client error")
			matchesDefaultConfig(t, actual)
		})

		t.Run("error loading toolchainconfig", func(t *testing.T) {
			// given
			config := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.AutomaticApproval().Enabled(true).MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)),
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member1", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(t, hostCl, members)
			count := 0
			hostCl.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.ToolchainConfig); ok {
					count++
					if count > 1 { // let the first get succeed and then return errors after that
						return fmt.Errorf("client error")
					}
				}
				return hostCl.Client.Get(ctx, key, obj)
			}

			// when
			res, err := controller.Reconcile(context.TODO(), newRequest())

			// then
			require.EqualError(t, err, "failed to load the latest configuration: client error")
			require.Empty(t, res)
			actual, err := toolchainconfig.GetToolchainConfig(hostCl)
			require.EqualError(t, err, "client error")
			matchesDefaultConfig(t, actual)
		})

		t.Run("reg service deploy failed", func(t *testing.T) {
			// given
			config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true).MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)), testconfig.Members().Default(defaultMemberConfig.Spec), testconfig.Members().SpecificPerMemberCluster("missing-member", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			hostCl.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
				return fmt.Errorf("create error")
			}
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(t, hostCl, members)

			// when
			_, err := controller.Reconcile(context.TODO(), newRequest())

			// then
			require.EqualError(t, err, "failed to apply registration service object registration-service: unable to create resource of kind: ServiceAccount, version: v1: create error")
			actual, err := toolchainconfig.GetToolchainConfig(hostCl)
			require.NoError(t, err)
			assert.True(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 80, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
			testconfig.AssertThatToolchainConfig(t, test.HostOperatorNs, hostCl).
				Exists().
				HasConditions(
					toolchainconfig.ToRegServiceDeployFailure("failed to apply registration service object registration-service: unable to create resource of kind: ServiceAccount, version: v1: create error")).
				HasNoSyncErrors()
		})

		t.Run("sync failed", func(t *testing.T) {
			// given
			config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true).MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)), testconfig.Members().Default(defaultMemberConfig.Spec), testconfig.Members().SpecificPerMemberCluster("missing-member", specificMemberConfig.Spec))
			hostCl := test.NewFakeClient(t, config)
			members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
			controller := newController(t, hostCl, members)

			// when
			res, err := controller.Reconcile(context.TODO(), newRequest())

			// then
			require.NoError(t, err)
			require.Equal(t, toolchainconfig.DefaultReconcile, res)
			actual, err := toolchainconfig.GetToolchainConfig(hostCl)
			require.NoError(t, err)
			assert.True(t, actual.AutomaticApproval().IsEnabled())
			assert.Equal(t, 123, actual.AutomaticApproval().MaxNumberOfUsersOverall())
			assert.Equal(t, config.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
			assert.Equal(t, 80, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
			testconfig.AssertThatToolchainConfig(t, test.HostOperatorNs, hostCl).
				Exists().
				HasConditions(
					toolchainconfig.ToSyncFailure(),
					toolchainconfig.ToRegServiceDeploying("updated resources: [ServiceAccount: registration-service Role: registration-service RoleBinding: registration-service Deployment: registration-service Service: registration-service Route: registration-service]")).
				HasSyncErrors(map[string]string{"missing-member": "specific member configuration exists but no matching toolchaincluster was found"})
		})
	})
}

func TestWrapErrorWithUpdateStatus(t *testing.T) {
	// given
	config := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true).MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)))
	hostCl := test.NewFakeClient(t, config)
	members := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
	controller := newController(t, hostCl, members)
	log := logf.Log.WithName("test")

	t.Run("no error provided", func(t *testing.T) {
		statusUpdater := func(toolchainConfig *toolchainv1alpha1.ToolchainConfig, message string) error {
			assert.Equal(t, "failed to load the latest configuration: underlying error", message)
			return nil
		}

		// test
		err := controller.WrapErrorWithStatusUpdate(log, config, statusUpdater, nil, "failed to load the latest configuration")

		require.Nil(t, err)
	})

	t.Run("status updated", func(t *testing.T) {
		statusUpdater := func(toolchainConfig *toolchainv1alpha1.ToolchainConfig, message string) error {
			assert.Equal(t, "failed to load the latest configuration: underlying error", message)
			return nil
		}

		// test
		err := controller.WrapErrorWithStatusUpdate(log, config, statusUpdater, fmt.Errorf("underlying error"), "failed to load the latest configuration")

		require.EqualError(t, err, "failed to load the latest configuration: underlying error")
	})

	t.Run("status update failed", func(t *testing.T) {
		statusUpdater := func(toolchainConfig *toolchainv1alpha1.ToolchainConfig, message string) error {
			return fmt.Errorf("unable to update status")
		}

		// when
		err := controller.WrapErrorWithStatusUpdate(log, config, statusUpdater, fmt.Errorf("underlying error"), "failed to load the latest configuration")

		// then
		require.EqualError(t, err, "failed to load the latest configuration: underlying error")
	})
}

func newRequest() reconcile.Request {
	return reconcile.Request{
		NamespacedName: test.NamespacedName(test.HostOperatorNs, "config"),
	}
}

func matchesDefaultConfig(t *testing.T, actual toolchainconfig.ToolchainConfig) {
	assert.False(t, actual.AutomaticApproval().IsEnabled())
	assert.Equal(t, 1000, actual.AutomaticApproval().MaxNumberOfUsersOverall())
	assert.Empty(t, actual.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
	assert.Equal(t, 80, actual.AutomaticApproval().ResourceCapacityThresholdDefault())
	assert.Empty(t, actual.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	assert.Equal(t, 3, actual.Deactivation().DeactivatingNotificationDays())
}

func newController(t *testing.T, hostCl client.Client, members cluster.GetMemberClustersFunc) toolchainconfig.Reconciler {
	os.Setenv("WATCH_NAMESPACE", test.HostOperatorNs)
	s := runtime.NewScheme()
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	err = clientgoscheme.AddToScheme(s)
	require.NoError(t, err)

	regServiceTemplate, err := registrationservice.GetDeploymentTemplate()
	require.NoError(t, err)

	return toolchainconfig.Reconciler{
		Client:             hostCl,
		GetMembersFunc:     members,
		Scheme:             s,
		RegServiceTemplate: regServiceTemplate,
	}
}

func getMemberConfig(cluster *cluster.CachedToolchainCluster) (*toolchainv1alpha1.MemberOperatorConfig, error) {
	memberConfig := &toolchainv1alpha1.MemberOperatorConfig{}
	err := cluster.Client.Get(context.TODO(), test.NamespacedName(cluster.OperatorNamespace, "config"), memberConfig)
	return memberConfig, err
}
