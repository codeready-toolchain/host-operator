package usersignup

import (
	"context"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
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
	config, err := configuration.LoadConfig(NewFakeClient(t))
	require.NoError(t, err)
	defer counter.Reset()
	InitializeCounter(t, 1500, UserAccountsForCluster("member1", 800), UserAccountsForCluster("member2", 700))
	toolchainStatus := NewToolchainStatus(
		WithMember("member1", WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)),
		WithMember("member2", WithNodeRoleUsage("worker", 55), WithNodeRoleUsage("master", 60)))
	signup := NewUserSignup()

	t.Run("with one cluster and enough capacity", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 70)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with two clusters and enough capacity in both of them so it returns the first one", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 1000), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 70), PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with two clusters where the first one reaches resource threshold", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 1000), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 60), PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters where the first one reaches max number of UserAccounts", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 6000), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 60), PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters, none of them is returned since it reaches max number of MURs", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(1200, PerMemberCluster("member1", 6000), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 60), PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Empty(t, clusterName)
	})

	t.Run("with two clusters and enough capacity only in second one using the default values", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000).
				ResourceCapThreshold(62))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("automatic approval not enabled and user not approved", func(t *testing.T) {
		// given
		fakeClient := NewFakeClient(t, toolchainStatus, NewHostOperatorConfig())
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Empty(t, clusterName)
	})

	t.Run("HostOperatorConfig not found and user not approved", func(t *testing.T) {
		// given
		fakeClient := NewFakeClient(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Empty(t, clusterName)
	})

	t.Run("HostOperatorConfig not found and user approved without target cluster", func(t *testing.T) {
		// given
		fakeClient := NewFakeClient(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
		signup := NewUserSignup(Approved())

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("automatic approval not enabled, user approved but no cluster has capacity", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().ResourceCapThreshold(50))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
		signup := NewUserSignup(Approved())

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.EqualError(t, err, "no suitable member cluster found")
		assert.False(t, approved)
		assert.Empty(t, clusterName)
	})

	t.Run("automatic approval not enabled, user approved and second cluster has capacity", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().
				MaxUsersNumber(2000).
				ResourceCapThreshold(62))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
		signup := NewUserSignup(Approved())

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("automatic approval not enabled, user approved, no cluster has capacity but targetCluster is specified", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().MaxUsersNumber(1000))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))
		signup := NewUserSignup(Approved(), WithTargetCluster("member1"))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with two clusters and enough capacity in both of them but first one is not ready", func(t *testing.T) {
		// given
		hostOperatorConfig := NewHostOperatorConfig(
			AutomaticApproval().
				Enabled().
				MaxUsersNumber(2000, PerMemberCluster("member1", 1000), PerMemberCluster("member2", 1000)).
				ResourceCapThreshold(80, PerMemberCluster("member1", 70), PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, hostOperatorConfig)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionFalse), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("failures", func(t *testing.T) {
		t.Run("unable to read HostOperatorConfig", func(t *testing.T) {
			// given
			fakeClient := NewFakeClient(t, toolchainStatus)
			fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				return fmt.Errorf("some error")
			}
			clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

			// when
			approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

			// then
			require.EqualError(t, err, "unable to read HostOperatorConfig resource: some error")
			assert.False(t, approved)
			assert.Empty(t, clusterName)
		})

		t.Run("unable to read ToolchainStatus", func(t *testing.T) {
			// given
			fakeClient := NewFakeClient(t, toolchainStatus, NewHostOperatorConfig(AutomaticApproval().Enabled()))
			fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				if _, ok := obj.(*v1alpha1.ToolchainStatus); ok {
					return fmt.Errorf("some error")
				}
				return fakeClient.Client.Get(ctx, key, obj)
			}
			clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

			// when
			approved, clusterName, err := getClusterIfApproved(fakeClient, config, signup, clusters)

			// then
			require.EqualError(t, err, "unable to read ToolchainStatus resource: some error")
			assert.False(t, approved)
			assert.Empty(t, clusterName)
		})
	})
}

func TestGetClusterIfApprovedWhenCounterIsNotInitialized(t *testing.T) {
	// given
	config, err := configuration.LoadConfig(NewFakeClient(t))
	require.NoError(t, err)
	toolchainStatus := NewToolchainStatus(
		WithMember("member1", WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)))
	fakeClient := NewFakeClient(t, toolchainStatus, NewHostOperatorConfig(AutomaticApproval().Enabled()))
	clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

	// when
	approved, clusterName, err := getClusterIfApproved(fakeClient, config, NewUserSignup(), clusters)

	// then
	require.EqualError(t, err, "unable to get the number of provisioned users: counter is not initialized")
	assert.False(t, approved)
	assert.Empty(t, clusterName)
}
