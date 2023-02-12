package usersignup

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetClusterIfApproved(t *testing.T) {
	// given
	signup := commonsignup.NewUserSignup()
	toolchainStatus := NewToolchainStatus(
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
			string(metrics.Internal): 100,
			string(metrics.External): 800,
		}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
			"1,internal": 200,
			"1,external": 700,
		}),
		WithMember("member1", WithSpaceCount(700), WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)),
		WithMember("member2", WithSpaceCount(200), WithNodeRoleUsage("worker", 55), WithNodeRoleUsage("master", 60)))

	t.Run("with two clusters available, the second one is defined as the last-used one", func(t *testing.T) {
		// given
		signup := commonsignup.NewUserSignup()
		signup.Annotations = map[string]string{
			toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey: "member2",
		}
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.AutomaticApproval().
				Enabled(true),
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000), testconfig.PerMemberCluster("member2", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 70), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue), NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("with two clusters available, the second one has required cluster-role label", func(t *testing.T) {
		// given
		signup := commonsignup.NewUserSignup()
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.AutomaticApproval().
				Enabled(true),
		)
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(
			NewMemberClusterWithoutClusterRoles(t, "member1", corev1.ConditionTrue), // no cluster-role label on this member
			NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue),      // by default all member clusters will have the 'tenant' cluster-role
		)

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("don't return preferred cluster name if without required cluster role", func(t *testing.T) {
		// given
		signup := commonsignup.NewUserSignup()
		signup.Annotations = map[string]string{
			toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey: "member1", // set preferred member cluster name
		}
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.AutomaticApproval().
				Enabled(true))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(
			// member1 doesn't have the cluster-role tenant but it's preferred one
			NewMemberClusterWithoutClusterRoles(t, "member1", corev1.ConditionTrue),
			NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue), // by default all member clusters will have the 'tenant' cluster-role
		)

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("with two clusters available, the second one has required cluster-role label", func(t *testing.T) {
		// given
		signup := commonsignup.NewUserSignup()
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.AutomaticApproval().
				Enabled(true),
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000), testconfig.PerMemberCluster("member2", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 70), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(
			NewMemberClusterWithoutClusterRoles(t, "member1", corev1.ConditionTrue), // no cluster-role label on this member
			NewMemberCluster(t, "member2", corev1.ConditionTrue),                    // by default all member clusters will have the 'tenant' cluster-role
		)

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("return preferred cluster name even if without required cluster role", func(t *testing.T) {
		// given
		signup := commonsignup.NewUserSignup()
		signup.Annotations = map[string]string{
			toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey: "member1", // set preferred member cluster name
		}
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.AutomaticApproval().
				Enabled(true),
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000), testconfig.PerMemberCluster("member2", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 70), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(
			// member1 doesn't have the cluster-role tenant but it's preferred one
			NewMemberClusterWithoutClusterRoles(t, "member1", corev1.ConditionTrue),
			NewMemberCluster(t, "member1", corev1.ConditionTrue), // by default all member clusters will have the 'tenant' cluster-role
		)

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName.getClusterName())
	})

	t.Run("with no cluster available", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.AutomaticApproval().
				Enabled(true))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters()

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, notFound, clusterName)
	})

	t.Run("automatic approval not enabled and user not approved", func(t *testing.T) {
		// given
		fakeClient := NewFakeClient(t, toolchainStatus, commonconfig.NewToolchainConfigObjWithReset(t))
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue), NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, unknown, clusterName)
	})

	t.Run("ToolchainConfig not found and user not approved", func(t *testing.T) {
		// given
		fakeClient := NewFakeClient(t, toolchainStatus)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue), NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, unknown, clusterName)
	})

	t.Run("ToolchainConfig not found and user manually approved without target cluster", func(t *testing.T) {
		// given
		fakeClient := NewFakeClient(t, toolchainStatus)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue), NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))
		signup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName.getClusterName())
	})

	t.Run("automatic approval not enabled, user manually approved but no cluster has capacity", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().ResourceCapacityThreshold(50),
		)
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue), NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))
		signup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, notFound, clusterName)
	})

	t.Run("automatic approval not enabled, user manually approved and second cluster has capacity", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 2000)).
				ResourceCapacityThreshold(62))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue), NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))
		signup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("automatic approval not enabled, user manually approved, no cluster has capacity but targetCluster is specified", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue), NewMemberClusterWithTenantRole(t, "member2", corev1.ConditionTrue))
		signup := commonsignup.NewUserSignup(commonsignup.ApprovedManually(), commonsignup.WithTargetCluster("member1"))

		// when
		approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName.getClusterName())
	})

	t.Run("failures", func(t *testing.T) {
		t.Run("unable to get ToolchainConfig", func(t *testing.T) {
			// given
			fakeClient := NewFakeClient(t, toolchainStatus)
			InitializeCounters(t, toolchainStatus)
			fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				return fmt.Errorf("some error")
			}
			InitializeCounters(t, toolchainStatus)
			clusters := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))

			// when
			approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

			// then
			require.EqualError(t, err, "unable to get ToolchainConfig: some error")
			assert.False(t, approved)
			assert.Equal(t, unknown, clusterName)
		})

		t.Run("unable to read ToolchainStatus", func(t *testing.T) {
			// given
			fakeClient := NewFakeClient(t, toolchainStatus, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
			fakeClient.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if _, ok := obj.(*toolchainv1alpha1.ToolchainStatus); ok {
					return fmt.Errorf("some error")
				}
				return fakeClient.Client.Get(ctx, key, obj)
			}
			InitializeCounters(t, toolchainStatus)
			clusters := NewGetMemberClusters(NewMemberClusterWithTenantRole(t, "member1", corev1.ConditionTrue))

			// when
			approved, clusterName, err := getClusterIfApproved(fakeClient, signup, capacity.NewClusterManager(clusters, fakeClient))

			// then
			require.EqualError(t, err, "unable to get the optimal target cluster: unable to read ToolchainStatus resource: some error")
			assert.False(t, approved)
			assert.Equal(t, unknown, clusterName)
		})
	})
}
