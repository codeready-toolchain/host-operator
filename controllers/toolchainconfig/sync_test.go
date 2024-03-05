package toolchainconfig_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	. "github.com/codeready-toolchain/host-operator/test"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSyncMemberConfigs(t *testing.T) {
	defaultMemberConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("5s"))
	specificMemberConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("10s"))

	t.Run("sync success", func(t *testing.T) {

		t.Run("no member clusters available - skip sync", func(t *testing.T) {
			// given
			toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member2", specificMemberConfig.Spec))
			s := toolchainconfig.NewSynchronizer(
				ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
				NewGetMemberClusters(),
			)

			// when
			syncErrors := s.SyncMemberConfigs(context.TODO(), toolchainConfig)

			// then
			require.Empty(t, syncErrors)
		})

		t.Run("synced to all members", func(t *testing.T) {
			// given
			toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member2", specificMemberConfig.Spec))
			s := toolchainconfig.NewSynchronizer(
				ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
				NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue), NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue)),
			)

			// when
			syncErrors := s.SyncMemberConfigs(context.TODO(), toolchainConfig)

			// then
			require.Empty(t, syncErrors)
		})
	})

	t.Run("sync fails", func(t *testing.T) {

		t.Run("sync to a member failed", func(t *testing.T) {
			// given
			memberCl := test.NewFakeClient(t)
			memberCl.MockGet = func(_ context.Context, _ types.NamespacedName, _ runtimeclient.Object, _ ...runtimeclient.GetOption) error {
				return fmt.Errorf("client error")
			}
			toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member2", specificMemberConfig.Spec))
			s := toolchainconfig.NewSynchronizer(
				ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
				NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue), NewMemberClusterWithClient(memberCl, "member2", corev1.ConditionTrue)),
			)

			// when
			syncErrors := s.SyncMemberConfigs(context.TODO(), toolchainConfig)

			// then
			require.Len(t, syncErrors, 1)
			assert.Equal(t, syncErrors["member2"], "client error")
		})

		t.Run("sync to multiple members failed", func(t *testing.T) {
			// given
			memberCl := test.NewFakeClient(t)
			memberCl.MockGet = func(_ context.Context, _ types.NamespacedName, _ runtimeclient.Object, _ ...runtimeclient.GetOption) error {
				return fmt.Errorf("client error")
			}
			memberCl2 := test.NewFakeClient(t)
			memberCl2.MockGet = func(_ context.Context, _ types.NamespacedName, _ runtimeclient.Object, _ ...runtimeclient.GetOption) error {
				return fmt.Errorf("client2 error")
			}
			toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member2", specificMemberConfig.Spec))
			s := toolchainconfig.NewSynchronizer(
				ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
				NewGetMemberClusters(NewMemberClusterWithClient(memberCl, "member1", corev1.ConditionTrue), NewMemberClusterWithClient(memberCl2, "member2", corev1.ConditionTrue)),
			)

			// when
			syncErrors := s.SyncMemberConfigs(context.TODO(), toolchainConfig)

			// then
			require.Len(t, syncErrors, 2)
			assert.Equal(t, syncErrors["member1"], "client error")
			assert.Equal(t, syncErrors["member2"], "client2 error")
		})

		t.Run("specific memberoperatorconfig exists but member cluster not found", func(t *testing.T) {
			// given
			memberCl := test.NewFakeClient(t)
			toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.Members().Default(defaultMemberConfig.Spec),
				testconfig.Members().SpecificPerMemberCluster("member2", specificMemberConfig.Spec))
			s := toolchainconfig.NewSynchronizer(
				ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
				NewGetMemberClusters(NewMemberClusterWithClient(memberCl, "member1", corev1.ConditionTrue)),
			)

			// when
			syncErrors := s.SyncMemberConfigs(context.TODO(), toolchainConfig)

			// then
			require.Len(t, syncErrors, 1)
			assert.Equal(t, syncErrors["member2"], "specific member configuration exists but no matching toolchaincluster was found")
		})
	})
}

func TestSyncMemberConfig(t *testing.T) {
	t.Run("sync success", func(t *testing.T) {

		t.Run("memberoperatorconfig created", func(t *testing.T) {
			// given
			memberCl := test.NewFakeClient(t)
			memberCluster := NewMemberClusterWithClient(memberCl, "member1", corev1.ConditionTrue)
			memberConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("5s"))

			// when
			err := toolchainconfig.SyncMemberConfig(context.TODO(), memberConfig.Spec, memberCluster)

			// then
			require.NoError(t, err)
			actual := &toolchainv1alpha1.MemberOperatorConfig{}
			err = memberCl.Get(context.TODO(), types.NamespacedName{Name: "config", Namespace: memberCluster.OperatorNamespace}, actual)
			require.NoError(t, err)
			require.Equal(t, memberConfig.Spec, actual.Spec)
		})

		t.Run("memberoperatorconfig updated", func(t *testing.T) {
			// given
			originalConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("10s"))
			memberCl := test.NewFakeClient(t, originalConfig)
			memberCluster := NewMemberClusterWithClient(memberCl, "member1", corev1.ConditionTrue)
			memberConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("5s"))

			// when
			err := toolchainconfig.SyncMemberConfig(context.TODO(), memberConfig.Spec, memberCluster)

			// then
			require.NoError(t, err)
			actual := &toolchainv1alpha1.MemberOperatorConfig{}
			err = memberCl.Get(context.TODO(), types.NamespacedName{Name: "config", Namespace: memberCluster.OperatorNamespace}, actual)
			require.NoError(t, err)
			require.Equal(t, memberConfig.Spec, actual.Spec)
		})
	})

	t.Run("sync fails", func(t *testing.T) {
		t.Run("client get error", func(t *testing.T) {
			// given
			memberCl := test.NewFakeClient(t)
			memberCl.MockGet = func(_ context.Context, key types.NamespacedName, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				return fmt.Errorf("client error")
			}
			memberCluster := NewMemberClusterWithClient(memberCl, "member1", corev1.ConditionTrue)
			memberConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("5s"))

			// when
			err := toolchainconfig.SyncMemberConfig(context.TODO(), memberConfig.Spec, memberCluster)

			// then
			require.EqualError(t, err, "client error")
			actual := &toolchainv1alpha1.MemberOperatorConfig{}
			err = memberCl.Get(context.TODO(), types.NamespacedName{Name: "config", Namespace: memberCluster.OperatorNamespace}, actual)
			require.Error(t, err)
		})

		t.Run("client update error", func(t *testing.T) {
			// given
			originalConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("10s"))
			memberCl := test.NewFakeClient(t, originalConfig)
			memberCl.MockUpdate = func(_ context.Context, _ runtimeclient.Object, _ ...runtimeclient.UpdateOption) error {
				return fmt.Errorf("client update error")
			}
			memberCluster := NewMemberClusterWithClient(memberCl, "member1", corev1.ConditionTrue)
			memberConfig := testconfig.NewMemberOperatorConfigObj(testconfig.MemberStatus().RefreshPeriod("5s"))

			// when
			err := toolchainconfig.SyncMemberConfig(context.TODO(), memberConfig.Spec, memberCluster)

			// then
			require.EqualError(t, err, "client update error")
			actual := &toolchainv1alpha1.MemberOperatorConfig{}
			err = memberCl.Get(context.TODO(), types.NamespacedName{Name: "config", Namespace: memberCluster.OperatorNamespace}, actual)
			require.NoError(t, err)
			require.Equal(t, originalConfig.Spec, actual.Spec) // should still have the original config
		})
	})
}
