package capacity_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	. "github.com/codeready-toolchain/host-operator/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetOptimalTargetCluster(t *testing.T) {
	// given
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
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 70)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)

		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with three clusters and enough capacity in all of them so it returns the with more capacity (the first one)", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 10000), testconfig.PerMemberCluster("member2", 300), testconfig.PerMemberCluster("member3", 400)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 70), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue), NewMemberCluster(t, "member3", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with three clusters and enough capacity in all of them so it returns the with more capacity (the third one)", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000), testconfig.PerMemberCluster("member2", 1000), testconfig.PerMemberCluster("member3", 2000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 70), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue), NewMemberCluster(t, "member3", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: HostOperatorNs,
			})

		// then
		require.NoError(t, err)
		assert.Equal(t, "member3", clusterName)
	})

	t.Run("with two clusters and enough capacity in both of them, but the second one is the preferred", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000), testconfig.PerMemberCluster("member2", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 70), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				PreferredCluster:         "member2",
				ToolchainStatusNamespace: HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters where the first one reaches resource threshold", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000), testconfig.PerMemberCluster("member2", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 60), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters where the first one reaches max number of Spaces, so the second one is returned even when the first is defined as the preferred one", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 700), testconfig.PerMemberCluster("member2", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 90), testconfig.PerMemberCluster("member2", 95)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				PreferredCluster:         "member1",
				ToolchainStatusNamespace: HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters, none of them is returned since it reaches max number of Spaces, no matter what is defined as preferred", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1), testconfig.PerMemberCluster("member2", 1)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 60), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				PreferredCluster:         "member2",
				ToolchainStatusNamespace: HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "", clusterName)
	})

	t.Run("with two clusters but only the second one has enough capacity - using the default values", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces().
				ResourceCapacityThreshold(62))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters but none of them has enough capacity - using the default memory values", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces().
				ResourceCapacityThreshold(1))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "", clusterName)
	})

	t.Run("with two clusters and enough capacity in both of them but first one is not ready", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000), testconfig.PerMemberCluster("member2", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 70), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionFalse), NewMemberCluster(t, "member2", v1.ConditionTrue))

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: HostOperatorNs,
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("with two clusters and enough capacity in both of them but passing specific cluster-role label", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1000), testconfig.PerMemberCluster("member2", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 70), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(
			NewMemberCluster(t, "member1", v1.ConditionTrue),
			NewMemberClusterWithClient(commontest.NewFakeClient(t), "member2", v1.ConditionTrue), // member2 has capacity but doesn't have the required cluster role
		)

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: HostOperatorNs,
				ClusterRoles:             []string{cluster.RoleLabel(cluster.Tenant)},
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "member1", clusterName) // only member one has required label
	})

	t.Run("with two clusters and not enough capacity on the cluster with specific cluster-role label", func(t *testing.T) {
		// given
		toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.CapacityThresholds().
				MaxNumberOfSpaces(testconfig.PerMemberCluster("member1", 1), testconfig.PerMemberCluster("member2", 1000)).
				ResourceCapacityThreshold(80, testconfig.PerMemberCluster("member1", 1), testconfig.PerMemberCluster("member2", 75)))
		fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
		InitializeCounters(t, toolchainStatus)
		clusters := NewGetMemberClusters(
			NewMemberCluster(t, "member1", v1.ConditionTrue),
			NewMemberClusterWithClient(commontest.NewFakeClient(t), "member2", v1.ConditionTrue), // member2 has capacity but doesn't have the required cluster role
		)

		// when
		clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
			&capacity.OptimalTargetClusterFilter{
				ToolchainStatusNamespace: HostOperatorNs,
				ClusterRoles:             []string{cluster.RoleLabel(cluster.Tenant)},
			},
		)

		// then
		require.NoError(t, err)
		assert.Equal(t, "", clusterName) // only member one has required label
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
			clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

			// when
			clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
				&capacity.OptimalTargetClusterFilter{
					ToolchainStatusNamespace: HostOperatorNs,
				},
			)

			// then
			require.EqualError(t, err, "unable to get ToolchainConfig: some error")
			assert.Equal(t, "", clusterName)
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
			clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

			// when
			clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
				&capacity.OptimalTargetClusterFilter{
					ToolchainStatusNamespace: HostOperatorNs,
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
	for _, limit := range []int{800, 1000, 1234, 2500, 10000} {
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
					toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t,
						testconfig.CapacityThresholds().
							MaxNumberOfSpaces(testconfig.PerMemberCluster("member2", limit), testconfig.PerMemberCluster("member3", limit)))

					fakeClient := NewFakeClient(t, toolchainStatus, toolchainConfig)
					InitializeCounters(t, toolchainStatus)
					clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue), NewMemberCluster(t, "member2", v1.ConditionTrue), NewMemberCluster(t, "member3", v1.ConditionTrue))
					clusterBalancer := capacity.NewClusterManager(clusters, fakeClient)

					// now run in 4 cycles and expect that the users will be provisioned in batches of 50
					member2CurrentCount := numberOfSpaces
					member3CurrentCount := numberOfSpaces
					for cycle := 0; cycle < 4; cycle++ {

						member2MissingTo50 := 50 - member2CurrentCount%50
						// this 50 users should go into member2 - it will be always 50
						for i := 0; i < member2MissingTo50; i++ {
							t.Run(fmt.Sprintf("cycle %d user %d for member2", cycle, i), func(t *testing.T) {
								//given
								// even when the counter of the other member is decremented, it should still use the last used one
								// but we can decrement it only in the second cycle when the member3 has at least 50 Spaces
								if i == 2 && cycle > 1 {
									counter.DecrementSpaceCount(log.Log, "member3")
									member3CurrentCount--
								}

								// when
								clusterName, err := clusterBalancer.GetOptimalTargetCluster(
									&capacity.OptimalTargetClusterFilter{
										ToolchainStatusNamespace: HostOperatorNs,
									},
								)

								// then
								require.NoError(t, err)
								assert.Equal(t, "member2", clusterName)

								counter.IncrementSpaceCount(log.Log, "member2")
								member2CurrentCount++
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
									&capacity.OptimalTargetClusterFilter{
										ToolchainStatusNamespace: HostOperatorNs,
									},
								)

								// then
								require.NoError(t, err)
								assert.Equal(t, "member3", clusterName)

								counter.IncrementSpaceCount(log.Log, "member3")
								member3CurrentCount++
							})
						}

					}

					// when
					clusterName, err := clusterBalancer.GetOptimalTargetCluster(
						&capacity.OptimalTargetClusterFilter{
							ToolchainStatusNamespace: HostOperatorNs,
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
	fakeClient := NewFakeClient(t, toolchainStatus, commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true)))
	clusters := NewGetMemberClusters(NewMemberCluster(t, "member1", v1.ConditionTrue))

	// when
	clusterName, err := capacity.NewClusterManager(clusters, fakeClient).GetOptimalTargetCluster(
		&capacity.OptimalTargetClusterFilter{
			ToolchainStatusNamespace: HostOperatorNs,
		},
	)

	// then
	require.EqualError(t, err, "unable to get the number of provisioned spaces: counter is not initialized")
	assert.Equal(t, "", clusterName)
}
