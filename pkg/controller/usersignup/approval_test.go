package usersignup

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetClusterIfApproved(t *testing.T) {
	// given
	signup := NewUserSignup()
	toolchainStatus := NewToolchainStatus(
		WithHost(WithMasterUserRecordCount(1500)),
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 100,
			string(metrics.External): 1400,
		}),
		WithMetric(toolchainv1alpha1.UsersPerActivationMetricKey, toolchainv1alpha1.Metric{
			"1": 1500,
		}),
		WithMember("member1", WithUserAccountCount(800), WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)),
		WithMember("member2", WithUserAccountCount(700), WithNodeRoleUsage("worker", 55), WithNodeRoleUsage("master", 60)))

	t.Run("with one cluster and enough capacity", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 70)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)

		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName.getClusterName())
	})

	t.Run("with two clusters and enough capacity in both of them so it returns the first one", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 1000), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 70), PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName.getClusterName())
	})

	t.Run("with two clusters where the first one reaches resource threshold", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 1000), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 60), PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("with two clusters where the first one reaches max number of UserAccounts", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 800), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 90), PerMemberCluster("member2", 95)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("with two clusters, none of them is returned since it reaches max number of MURs", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(1200, PerMemberCluster("member1", 6000), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 60), PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, notFound, clusterName)
	})

	t.Run("with two clusters and enough capacity only in second one using the default values", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000).
				ResourceCapThreshold(62))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("with two clusters and enough capacity in none of them using the default memory values", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(5000).
				ResourceCapThreshold(1))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, notFound, clusterName)
	})

	t.Run("automatic approval not enabled and user not approved", func(t *testing.T) {
		// given
		fakeClient := NewFakeClient(t, toolchainStatus, NewHostOperatorConfigWithReset(t))
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, unknown, clusterName)
	})

	t.Run("HostOperatorConfig not found and user not approved", func(t *testing.T) {
		// given
		fakeClient := NewFakeClient(t, toolchainStatus)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, unknown, clusterName)
	})

	t.Run("HostOperatorConfig not found and user approved without target cluster", func(t *testing.T) {
		// given
		fakeClient := NewFakeClient(t, toolchainStatus)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
		signup := NewUserSignup(Approved())

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName.getClusterName())
	})

	t.Run("automatic approval not enabled, user approved but no cluster has capacity", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().ResourceCapThreshold(50))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
		signup := NewUserSignup(Approved())

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, notFound, clusterName)
	})

	t.Run("automatic approval not enabled, user approved and second cluster has capacity", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().
				MaxUsersNumber(2000).
				ResourceCapThreshold(62))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
		signup := NewUserSignup(Approved())

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("automatic approval not enabled, user approved, no cluster has capacity but targetCluster is specified", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().MaxUsersNumber(1000))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
		signup := NewUserSignup(Approved(), WithTargetCluster("member1"))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName.getClusterName())
	})

	t.Run("with two clusters and enough capacity in both of them but first one is not ready", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfigWithReset(t,
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 1000), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 70), PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionFalse), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("failures", func(t *testing.T) {
		t.Run("unable to read HostOperatorConfig", func(t *testing.T) {
			// given
			fakeClient := NewFakeClient(t, toolchainStatus)
			InitializeCounters(t, toolchainStatus)
			fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				return fmt.Errorf("some error")
			}
			InitializeCounters(t, toolchainStatus)
			clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

			// when
			approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

			// then
			require.EqualError(t, err, "unable to read HostOperatorConfig resource: some error")
			assert.False(t, approved)
			assert.Equal(t, unknown, clusterName)
		})

		t.Run("unable to read ToolchainStatus", func(t *testing.T) {
			// given
			fakeClient := NewFakeClient(t, toolchainStatus, NewHostOperatorConfigWithReset(t, AutomaticApproval().Enabled()))
			fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.ToolchainStatus); ok {
					return fmt.Errorf("some error")
				}
				return fakeClient.Client.Get(ctx, key, obj)
			}
			InitializeCounters(t, toolchainStatus)
			clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

			// when
			approved, clusterName, err := getClusterIfApproved(fakeClient, signup, clusters)

			// then
			require.EqualError(t, err, "unable to read ToolchainStatus resource: some error")
			assert.False(t, approved)
			assert.Equal(t, unknown, clusterName)
		})
	})
}

func TestGetClusterIfApprovedWhenCounterIsNotInitialized(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(
		WithMember("member1", WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)))
	fakeClient := NewFakeClient(t, toolchainStatus, NewHostOperatorConfigWithReset(t, AutomaticApproval().Enabled()))
	clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

	// when
	approved, clusterName, err := getClusterIfApproved(fakeClient, NewUserSignup(), clusters)

	// then
	require.EqualError(t, err, "unable to get the number of provisioned users: counter is not initialized")
	assert.False(t, approved)
	assert.Equal(t, unknown, clusterName)
}
