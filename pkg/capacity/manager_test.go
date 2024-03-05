package capacity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	spc "github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestGetOptimalTargetCluster(t *testing.T) {
	// given
	ctx := context.TODO()
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
		WithMember("member2", WithSpaceCount(200), WithNodeRoleUsage("worker", 55), WithNodeRoleUsage("master", 60)),
		WithMember("member3", WithSpaceCount(200), WithNodeRoleUsage("worker", 55), WithNodeRoleUsage("master", 50)))

	t.Run("with one cluster and enough capacity", func(t *testing.T) {
		// given
		spaceProvisionerConfig := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole(
			"member1Spc",
			commontest.HostOperatorNs,
			"member1",
			spc.MaxNumberOfSpaces(1000),
			spc.MaxMemoryUtilizationPercent(70))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spaceProvisionerConfig)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with three clusters and enough capacity in all of them so it returns the with more capacity (the first one)", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(10000), spc.MaxMemoryUtilizationPercent(70))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(300), spc.MaxMemoryUtilizationPercent(75))
		spc3 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member3Spc", commontest.HostOperatorNs, "member3", spc.MaxNumberOfSpaces(400), spc.MaxMemoryUtilizationPercent(80))

		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2, spc3)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with three clusters and enough capacity in all of them so it returns the with more capacity (the third one)", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(70))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(75))
		spc3 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member3Spc", commontest.HostOperatorNs, "member3", spc.MaxNumberOfSpaces(2000), spc.MaxMemoryUtilizationPercent(80))

		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2, spc3)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			})

		// then
		require.NoError(t, err)
		assert.Equal(t, "member3", clusterName)
	})

	t.Run("with two clusters and enough capacity in both of them, but the second one is the preferred", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(70))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(75))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				PreferredCluster:         "member2",
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters where the first one reaches resource threshold", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(60))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(75))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters where the first one reaches max number of Spaces, so the second one is returned even when the first is defined as the preferred one", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(700), spc.MaxMemoryUtilizationPercent(90))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(95))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				PreferredCluster:         "member1",
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters, none of them is returned since it reaches max number of Spaces, no matter what is defined as preferred", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(1), spc.MaxMemoryUtilizationPercent(60))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(1), spc.MaxMemoryUtilizationPercent(95))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				PreferredCluster:         "member2",
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "", clusterName)
	})

	t.Run("with two clusters but only the second one has enough capacity - using the default values", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxMemoryUtilizationPercent(62))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxMemoryUtilizationPercent(62))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters but none of them has enough capacity - using the default memory values", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxMemoryUtilizationPercent(1))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxMemoryUtilizationPercent(1))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "", clusterName)
	})

	t.Run("with two clusters and enough capacity in both of them but first one is not ready", func(t *testing.T) {
		// given
		spc1 := spc.NewSpaceProvisionerConfig("member1Spc", commontest.HostOperatorNs,
			spc.Enabled(false),
			spc.WithReadyConditionValid(),
			spc.WithPlacementRoles(spc.PlacementRole("tenant")),
			spc.ReferencingToolchainCluster("member1"),
			spc.MaxNumberOfSpaces(1000),
			spc.MaxMemoryUtilizationPercent(70))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(75))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters and enough capacity in both of them but passing specific placement-role", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(70))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithoutPlacementRoles("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(75))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: commontest.HostOperatorNs,
				ClusterRoles:             []string{cluster.RoleLabel(cluster.Tenant)},
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member1", clusterName) // only member one has required label
	})

	t.Run("with two clusters and not enough capacity on the cluster with specific placement-role", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(1), spc.MaxMemoryUtilizationPercent(1))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithoutPlacementRoles("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(75))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: commontest.HostOperatorNs,
				ClusterRoles:             []string{cluster.RoleLabel(cluster.Tenant)},
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "", clusterName) // only member one has required label but no capacity
	})

	t.Run("with two clusters, the preferred one is returned if it has the required placement-roles", func(t *testing.T) {
		// given
		spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(70))
		spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(1000), spc.MaxMemoryUtilizationPercent(75))
		fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2)
		InitializeCounters(t, toolchainStatus)

		// when
		clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
			ctx,
			capacity.OptimalTargetClusterFilter{
				PreferredCluster:         "member2",                             // request specifically this member eve if it doesn't match the cluster-roles from below
				ClusterRoles:             []string{spc.PlacementRole("tenant")}, // set
				ToolchainStatusNamespace: commontest.HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("failures", func(t *testing.T) {
		t.Run("unable to list SpaceProvisionerConfigs", func(t *testing.T) {
			// given
			fakeClient := commontest.NewFakeClient(t, toolchainStatus)
			InitializeCounters(t, toolchainStatus)
			fakeClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceProvisionerConfigList); ok {
					return errors.New("some error")
				}
				return fakeClient.Client.List(ctx, list, opts...)
			}
			InitializeCounters(t, toolchainStatus)

			// when
			clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
				ctx,
				capacity.OptimalTargetClusterFilter{
					ToolchainStatusNamespace: commontest.HostOperatorNs,
				},
			)

			// then
			require.EqualError(t, err, "failed to find the optimal space provisioner config: some error")
			assert.Equal(t, "", clusterName)
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
			clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
				ctx,
				capacity.OptimalTargetClusterFilter{
					ToolchainStatusNamespace: commontest.HostOperatorNs,
				},
			)

			// then
			require.EqualError(t, err, "unable to read ToolchainStatus resource: some error")
			assert.Equal(t, "", clusterName)
		})
	})
}

func TestGetOptimalTargetClusterInBatchesBy50WhenTwoClusterHaveTheSameUsage(t *testing.T) {
	// given
	ctx := context.TODO()
	for _, limit := range []uint{800, 1000, 1234, 2500, 10000} {
		t.Run(fmt.Sprintf("for the given limit of max number of spaces per cluster: %d", limit), func(t *testing.T) {
			for _, numberOfSpaces := range []int{0, 8, 50, 88, 100, 123, 555} {
				t.Run(fmt.Sprintf("when there is a number of spaces at the very beginning %d", numberOfSpaces), func(t *testing.T) {
					toolchainStatus := NewToolchainStatus(
						WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{
							string(metrics.Internal): 1000,
						}),
						WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{
							"1,internal": 1000,
						}),
						WithMember("member1", WithSpaceCount(1000), WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)),
						WithMember("member2", WithSpaceCount(numberOfSpaces), WithNodeRoleUsage("worker", 55), WithNodeRoleUsage("master", 60)),
						WithMember("member3", WithSpaceCount(numberOfSpaces), WithNodeRoleUsage("worker", 55), WithNodeRoleUsage("master", 50)))

					// member2 and member3 have the same capacity left and the member1 is full, so no one can be provisioned there
					spc1 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member1Spc", commontest.HostOperatorNs, "member1", spc.MaxNumberOfSpaces(1000))
					spc2 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member2Spc", commontest.HostOperatorNs, "member2", spc.MaxNumberOfSpaces(limit))
					spc3 := spc.NewEnabledValidSpaceProvisionerConfigWithTenantRole("member3Spc", commontest.HostOperatorNs, "member3", spc.MaxNumberOfSpaces(limit))

					fakeClient := commontest.NewFakeClient(t, toolchainStatus, spc1, spc2, spc3)
					InitializeCounters(t, toolchainStatus)
					clusterBalancer := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

					// now run in 4 cycles and expect that the users will be provisioned in batches of 50
					member2CurrentCount := numberOfSpaces
					member3CurrentCount := numberOfSpaces
					for cycle := 0; cycle < 4; cycle++ {

						member2MissingTo50 := 50 - member2CurrentCount%50
						// this 50 users should go into member2 - it will be always 50
						for i := 0; i < member2MissingTo50; i++ {
							t.Run(fmt.Sprintf("cycle %d user %d for member2", cycle, i), func(t *testing.T) {
								// given
								// even when the counter of the other member is decremented, it should still use the last used one
								// but we can decrement it only in the second cycle when the member3 has at least 50 Spaces
								if i == 2 && cycle > 1 {
									counter.DecrementSpaceCount(log.Log, "member3")
									member3CurrentCount--
								}

								// when
								clusterName, err := clusterBalancer.GetOptimalTargetCluster(
									ctx,
									capacity.OptimalTargetClusterFilter{
										ToolchainStatusNamespace: commontest.HostOperatorNs,
									},
								)

								// then
								require.NoError(t, err)
								assert.Equal(t, "member2", clusterName)

								counter.IncrementSpaceCount(log.Log, "member2")
								member2CurrentCount++

								// and when calling it with the other cluster as preferred
								clusterName, err = clusterBalancer.GetOptimalTargetCluster(
									ctx,
									capacity.OptimalTargetClusterFilter{
										ToolchainStatusNamespace: commontest.HostOperatorNs,
										PreferredCluster:         "member3",
									},
								)

								// then it should return the preferred one, but it shouldn't have any effect on the "balancing and batching" logic in the following iteration.
								require.NoError(t, err)
								assert.Equal(t, "member3", clusterName)
							})
						}

						// reset the decremented counter back
						if member2MissingTo50 > 2 && cycle > 1 {
							counter.IncrementSpaceCount(log.Log, "member3")
							member3CurrentCount++
						}

						member3MissingTo50 := 50 - member3CurrentCount%50
						// this batch of users should go into member3 - the size of the batch depends on how many times the cluster is bigger than member2
						for i := 0; i < member3MissingTo50; i++ {
							t.Run(fmt.Sprintf("cycle %d user %d for member3", cycle, i), func(t *testing.T) {
								// when
								clusterName, err := clusterBalancer.GetOptimalTargetCluster(
									ctx,
									capacity.OptimalTargetClusterFilter{
										ToolchainStatusNamespace: commontest.HostOperatorNs,
									},
								)

								// then
								require.NoError(t, err)
								assert.Equal(t, "member3", clusterName)

								counter.IncrementSpaceCount(log.Log, "member3")
								member3CurrentCount++

								// and when calling it with the other cluster as preferred
								clusterName, err = clusterBalancer.GetOptimalTargetCluster(
									ctx,
									capacity.OptimalTargetClusterFilter{
										ToolchainStatusNamespace: commontest.HostOperatorNs,
										PreferredCluster:         "member2",
									},
								)

								// then it should return the preferred one, but it shouldn't have any effect on the "balancing and batching" logic in the following iteration.
								require.NoError(t, err)
								assert.Equal(t, "member2", clusterName)
							})
						}

					}

					// when
					clusterName, err := clusterBalancer.GetOptimalTargetCluster(
						ctx,
						capacity.OptimalTargetClusterFilter{
							ToolchainStatusNamespace: commontest.HostOperatorNs,
						},
					)

					// then
					require.NoError(t, err)
					// expect that it would start provisioning in member2 again
					assert.Equal(t, "member2", clusterName)
				})
			}
		})
	}
}

func TestGetOptimalTargetClusterWhenCounterIsNotInitialized(t *testing.T) {
	// given
	toolchainStatus := NewToolchainStatus(
		WithMember("member1", WithNodeRoleUsage("worker", 68), WithNodeRoleUsage("master", 65)))
	fakeClient := commontest.NewFakeClient(t, toolchainStatus, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))

	// when
	clusterName, err := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient).GetOptimalTargetCluster(
		context.TODO(),
		capacity.OptimalTargetClusterFilter{
			ToolchainStatusNamespace: commontest.HostOperatorNs,
		},
	)

	// then
	require.EqualError(t, err, "unable to get the number of provisioned spaces: counter is not initialized")
	assert.Equal(t, "", clusterName)
}
