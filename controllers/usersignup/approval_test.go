package usersignup

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	hspc "github.com/codeready-toolchain/host-operator/test/spaceprovisionerconfig"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	spc "github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetClusterIfApproved(t *testing.T) {
	// given
	ctx := context.TODO()
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
		)
		spc1 := hspc.NewEnabledValidTenantSPC("member1", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(70))
		spc2 := hspc.NewEnabledValidTenantSPC("member2", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(75))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, toolchainConfig, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("with two clusters available, the second one has required cluster-role label", func(t *testing.T) {
		// given
		signup := commonsignup.NewUserSignup()
		spc1 := hspc.NewEnabledValidSPC("member1")
		spc2 := hspc.NewEnabledValidTenantSPC("member2")
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.AutomaticApproval().
				Enabled(true),
		)
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, toolchainConfig, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	type TestFields struct {
		UserSignupEmail        string
		DomainConfiguartion    string
		ExpectedApprovedStatus bool
		ExpectedCluster        targetCluster
		ErrorExpected          bool
		ExpectedErrorMsg       string
	}

	domainsTests := map[string]TestFields{
		"single cluster, auto-approved domains is empty": {
			UserSignupEmail:        "tato@somedomain.org",
			DomainConfiguartion:    "",
			ExpectedApprovedStatus: true,
			ExpectedCluster:        "member1",
			ErrorExpected:          false,
			ExpectedErrorMsg:       "",
		},
		"single cluster, email domain not in auto-approved domains": {
			UserSignupEmail:        "tato@mydomain.org",
			DomainConfiguartion:    "somedomain.org,anotherdomain.edu",
			ExpectedApprovedStatus: false,
			ExpectedCluster:        unknown,
			ErrorExpected:          false,
			ExpectedErrorMsg:       "",
		},
		"single cluster, email domain in auto-approved domains": {
			UserSignupEmail:        "tato@somedomain.org",
			DomainConfiguartion:    "somedomain.org,anotherdomain.edu",
			ExpectedApprovedStatus: true,
			ExpectedCluster:        "member1",
			ErrorExpected:          false,
			ExpectedErrorMsg:       "",
		},
		"single cluster, invalid email format": {
			UserSignupEmail:        "tato@mydomain@somedomain.org",
			DomainConfiguartion:    "somedomain.org,anotherdomain.edu",
			ExpectedApprovedStatus: false,
			ExpectedCluster:        unknown,
			ErrorExpected:          true,
			ExpectedErrorMsg:       "unable to determine automatic approval: invalid email address: tato@mydomain@somedomain.org",
		},
	}

	for testName, testFields := range domainsTests {
		t.Run(testName, func(t *testing.T) {
			// given
			signup := commonsignup.NewUserSignup(commonsignup.WithEmail(testFields.UserSignupEmail))
			spc1 := hspc.NewEnabledValidTenantSPC("member1")
			toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
				testconfig.AutomaticApproval().
					Enabled(true).Domains(testFields.DomainConfiguartion),
			)
			fakeClient := commontest.NewFakeClient(t, toolchainStatus, toolchainConfig, spc1)
			InitializeCounters(t, toolchainStatus)

			// when
			approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

			// then
			if testFields.ErrorExpected {
				require.EqualError(t, err, testFields.ExpectedErrorMsg)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, testFields.ExpectedApprovedStatus, approved)
			assert.Equal(t, testFields.ExpectedCluster, clusterName)
		})
	}

	t.Run("don't return preferred cluster name if without required cluster role", func(t *testing.T) {
		// given
		signup := commonsignup.NewUserSignup()
		signup.Annotations = map[string]string{
			toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey: "member1", // set preferred member cluster name
		}
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.AutomaticApproval().
				Enabled(true))
		spc1 := hspc.NewEnabledValidSPC("member1")
		spc2 := hspc.NewEnabledValidTenantSPC("member2")
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, toolchainConfig, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("with no cluster available", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.AutomaticApproval().
				Enabled(true))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, notFound, clusterName)
	})

	t.Run("automatic approval not enabled and user not approved", func(t *testing.T) {
		// given
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		spc2 := hspc.NewEnabledValidTenantSPC("member2")
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, commonconfig.NewToolchainConfigObjWithReset(t), spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, unknown, clusterName)
	})

	t.Run("ToolchainConfig not found and user not approved", func(t *testing.T) {
		// given
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		spc2 := hspc.NewEnabledValidTenantSPC("member2")
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Equal(t, unknown, clusterName)
	})

	t.Run("ToolchainConfig not found and user manually approved without target cluster", func(t *testing.T) {
		// given
		spc1 := hspc.NewEnabledValidTenantSPC("member1")
		spc2 := hspc.NewEnabledValidTenantSPC("member2")
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)
		signup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName.getClusterName())
	})

	t.Run("automatic approval not enabled, user manually approved but no cluster has capacity", func(t *testing.T) {
		// given
		spc1 := hspc.NewEnabledValidTenantSPC("member1", spc.MaxMemoryUtilizationPercent(50))
		spc2 := hspc.NewEnabledValidTenantSPC("member2", spc.MaxMemoryUtilizationPercent(50))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)
		signup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, notFound, clusterName)
	})

	t.Run("automatic approval not enabled, user manually approved and second cluster has capacity", func(t *testing.T) {
		// given
		spc1 := hspc.NewEnabledValidTenantSPC("member1", spc.MaxNumberOfSpaces(2000), spc.MaxMemoryUtilizationPercent(62))
		spc2 := hspc.NewEnabledValidTenantSPC("member2", spc.MaxMemoryUtilizationPercent(62))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)
		signup := commonsignup.NewUserSignup(commonsignup.ApprovedManually())

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member2", clusterName.getClusterName())
	})

	t.Run("automatic approval not enabled, user manually approved, no cluster has capacity but targetCluster is specified", func(t *testing.T) {
		// given
		spc1 := hspc.NewEnabledValidTenantSPC("member1", spc.MaxNumberOfSpaces(1000))
		spc2 := hspc.NewEnabledValidTenantSPC("member2")
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)
		signup := commonsignup.NewUserSignup(commonsignup.ApprovedManually(), commonsignup.WithTargetCluster("member1"))

		// when
		approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

		// then
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Equal(t, "member1", clusterName.getClusterName())
	})

	t.Run("failures", func(t *testing.T) {
		t.Run("unable to get ToolchainConfig", func(t *testing.T) {
			// given
			fakeClient := commontest.NewFakeClient(t, toolchainStatus)
			fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				return fmt.Errorf("some error")
			}
			InitializeCounters(t, toolchainStatus)

			// when
			approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

			// then
			require.EqualError(t, err, "unable to get ToolchainConfig: some error")
			assert.False(t, approved)
			assert.Equal(t, unknown, clusterName)
		})

		t.Run("unable to read ToolchainStatus", func(t *testing.T) {
			// given
			fakeClient := commontest.NewFakeClient(t, toolchainStatus, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
			fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				if _, ok := obj.(*toolchainv1alpha1.ToolchainStatus); ok {
					return fmt.Errorf("some error")
				}
				return fakeClient.Client.Get(ctx, key, obj, opts...)
			}
			InitializeCounters(t, toolchainStatus)

			// when
			approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

			// then
			require.EqualError(t, err, "unable to get the optimal target cluster: unable to read ToolchainStatus resource: some error")
			assert.False(t, approved)
			assert.Equal(t, unknown, clusterName)
		})

		t.Run("unable to read SpaceProvisionerConfig", func(t *testing.T) {
			// given
			fakeClient := commontest.NewFakeClient(t, toolchainStatus, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
			fakeClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceProvisionerConfigList); ok {
					return fmt.Errorf("some error")
				}
				return fakeClient.Client.List(ctx, list, opts...)
			}
			InitializeCounters(t, toolchainStatus)

			// when
			approved, clusterName, err := getClusterIfApproved(ctx, fakeClient, signup, capacity.NewClusterManager(test.HostOperatorNs, fakeClient))

			// then
			require.EqualError(t, err, "unable to get the optimal target cluster: failed to find the optimal space provisioner config: some error")
			assert.False(t, approved)
			assert.Equal(t, unknown, clusterName)
		})
	})
}
