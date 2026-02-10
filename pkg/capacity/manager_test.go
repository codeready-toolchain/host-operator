package capacity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	metricstest "github.com/codeready-toolchain/host-operator/test/metrics"
	hspc "github.com/codeready-toolchain/host-operator/test/spaceprovisionerconfig"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	spc "github.com/codeready-toolchain/toolchain-common/pkg/test/spaceprovisionerconfig"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type memberConfig struct {
	clusterName string
	valid       bool
	hasRole     bool
	preferred   bool
}

func (m memberConfig) isCandidate() bool {
	return m.valid && m.hasRole
}

func (m memberConfig) buildSpaceProvisionerConfig() *toolchainv1alpha1.SpaceProvisionerConfig {
	opts := []spc.CreateOption{spc.ReferencingToolchainCluster(m.clusterName)}
	if m.valid {
		opts = append(opts, spc.WithReadyConditionValid())
	}
	if m.hasRole {
		opts = append(opts, spc.WithPlacementRoles(spc.PlacementRole("tenant")))
	}

	return spc.NewSpaceProvisionerConfig(m.clusterName, commontest.HostOperatorNs, opts...)
}

func generateMemberConfigCombinations(name string, preferred bool) []memberConfig {
	var combinations []memberConfig
	for _, valid := range []bool{false, true} {
		for _, hasRole := range []bool{false, true} {
			combinations = append(combinations, memberConfig{clusterName: name, valid: valid, hasRole: hasRole, preferred: preferred})
		}
	}
	return combinations
}

type getOptimalTargetClusterTestCase struct {
	member1 memberConfig
	member2 memberConfig
}

func (tc *getOptimalTargetClusterTestCase) getSelectedMemberName() string {
	var selected string
	if tc.member1.isCandidate() {
		selected = tc.member1.clusterName
		if tc.member2.isCandidate() && tc.member2.preferred {
			selected = tc.member2.clusterName
		}
	} else if tc.member2.isCandidate() {
		selected = tc.member2.clusterName
	}
	return selected
}

func (tc *getOptimalTargetClusterTestCase) getPreferredCluster() string {
	if tc.member1.preferred {
		return tc.member1.clusterName
	} else if tc.member2.preferred {
		return tc.member2.clusterName
	}
	return ""
}

func TestGetOptimalTargetCluster(t *testing.T) {
	var testCases []getOptimalTargetClusterTestCase

	// preferred == 0: no preferered cluster
	// preferred == 1: member1 preferred
	// preferred == 2: member2 preferred
	for preferred := 0; preferred < 3; preferred++ {
		member1s := generateMemberConfigCombinations("member1", preferred == 1)
		member2s := generateMemberConfigCombinations("member2", preferred == 2)

		for _, member1 := range member1s {
			for _, member2 := range member2s {
				testCases = append(testCases, getOptimalTargetClusterTestCase{member1: member1, member2: member2})
			}
		}
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("selecting between member1(%+v) and member2(%+v) should yield %s", tc.member1, tc.member2, tc.getSelectedMemberName()), func(t *testing.T) {
			// given
			spc1 := tc.member1.buildSpaceProvisionerConfig()
			spc2 := tc.member2.buildSpaceProvisionerConfig()
			metricstest.InitializeCountersWith(t) // no space counts necessary for this test
			fakeClient := commontest.NewFakeClient(t, spc1, spc2)
			cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

			// when
			clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{
				ClusterRoles:     []string{spc.PlacementRole("tenant")},
				PreferredCluster: tc.getPreferredCluster(),
			})

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.getSelectedMemberName(), clusterName)
		})
	}

	t.Run("with one cluster and enough capacity", func(t *testing.T) {
		// given
		spaceProvisionerConfig := hspc.NewEnabledValidTenantSPC("member1")
		fakeClient := commontest.NewFakeClient(t, spaceProvisionerConfig)
		metricstest.InitializeCountersWith(t)
		cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

		// when
		clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{})

		// then
		require.NoError(t, err)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with three eligible clusters it returns the one with the most capacity (the first one)", func(t *testing.T) {
		// given
		spc1 := hspc.NewEnabledValidTenantSPC("member1", spc.MaxNumberOfSpaces(10000))
		spc2 := hspc.NewEnabledValidTenantSPC("member2", spc.MaxNumberOfSpaces(1000))
		spc3 := hspc.NewEnabledValidTenantSPC("member3", spc.MaxNumberOfSpaces(1000))
		metricstest.InitializeCountersWith(t,
			metricstest.ClusterCount("member1", 700),
			metricstest.ClusterCount("member2", 700),
			metricstest.ClusterCount("member3", 200))
		fakeClient := commontest.NewFakeClient(t, spc1, spc2, spc3)
		cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

		// when
		clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{})

		// then
		require.NoError(t, err)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with three eligible clusters it returns the one with the most capacity (the third one)", func(t *testing.T) {
		// given
		spc1 := hspc.NewEnabledValidTenantSPC("member1", spc.MaxNumberOfSpaces(1000))
		spc2 := hspc.NewEnabledValidTenantSPC("member2", spc.MaxNumberOfSpaces(1000))
		spc3 := hspc.NewEnabledValidTenantSPC("member3", spc.MaxNumberOfSpaces(1000))

		metricstest.InitializeCountersWith(t,
			metricstest.ClusterCount("member1", 700),
			metricstest.ClusterCount("member2", 700),
			metricstest.ClusterCount("member3", 200))

		fakeClient := commontest.NewFakeClient(t, spc1, spc2, spc3)

		cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

		// when
		clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{})

		// then
		require.NoError(t, err)
		assert.Equal(t, "member3", clusterName)
	})

	t.Run("choose one of the configured clusters because the preferred one is missing the SPC", func(t *testing.T) {
		// given
		spc1 := hspc.NewEnabledValidTenantSPC("member1", spc.MaxNumberOfSpaces(1000))
		spc2 := hspc.NewEnabledValidTenantSPC("member2", spc.MaxNumberOfSpaces(1000))
		metricstest.InitializeCountersWith(t,
			metricstest.ClusterCount("member1", 700),
			metricstest.ClusterCount("member2", 500))
		fakeClient := commontest.NewFakeClient(t, spc1, spc2)
		cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

		// when
		clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{
			PreferredCluster: "member3",                             // request specifically this member eve if it doesn't match the cluster-roles from below
			ClusterRoles:     []string{spc.PlacementRole("tenant")}, // set
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "member2", clusterName)
	})

	t.Run("failures", func(t *testing.T) {
		t.Run("unable to list SpaceProvisionerConfigs", func(t *testing.T) {
			// given
			metricstest.InitializeCountersWith(t)
			fakeClient := commontest.NewFakeClient(t)
			fakeClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				if _, ok := list.(*toolchainv1alpha1.SpaceProvisionerConfigList); ok {
					return errors.New("some error")
				}
				return fakeClient.Client.List(ctx, list, opts...)
			}
			cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

			// when
			clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{})

			// then
			require.EqualError(t, err, "failed to find the optimal space provisioner config: some error")
			assert.Empty(t, clusterName)
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
					// member2 and member3 have the same capacity left and the member1 is full, so no one can be provisioned there
					spc1 := hspc.NewEnabledValidTenantSPC("member1",
						spc.MaxNumberOfSpaces(1000))
					spc2 := hspc.NewEnabledValidTenantSPC("member2",
						spc.MaxNumberOfSpaces(limit))
					spc3 := hspc.NewEnabledValidTenantSPC("member3",
						spc.MaxNumberOfSpaces(limit))

					metricstest.InitializeCountersWith(t,
						metricstest.ClusterCount("member1", 1000),
						metricstest.ClusterCount("member2", numberOfSpaces),
						metricstest.ClusterCount("member3", numberOfSpaces))

					fakeClient := commontest.NewFakeClient(t, spc1, spc2, spc3)
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
								clusterName, err := clusterBalancer.GetOptimalTargetCluster(ctx, capacity.OptimalTargetClusterFilter{})

								// then
								require.NoError(t, err)
								assert.Equal(t, "member2", clusterName)

								counter.IncrementSpaceCount(log.Log, "member2")
								member2CurrentCount++

								// and when calling it with the other cluster as preferred
								clusterName, err = clusterBalancer.GetOptimalTargetCluster(ctx, capacity.OptimalTargetClusterFilter{
									PreferredCluster: "member3",
								})

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
									capacity.OptimalTargetClusterFilter{},
								)

								// then
								require.NoError(t, err)
								assert.Equal(t, "member3", clusterName)

								counter.IncrementSpaceCount(log.Log, "member3")
								member3CurrentCount++

								// and when calling it with the other cluster as preferred
								clusterName, err = clusterBalancer.GetOptimalTargetCluster(ctx, capacity.OptimalTargetClusterFilter{
									PreferredCluster: "member2",
								})

								// then it should return the preferred one, but it shouldn't have any effect on the "balancing and batching" logic in the following iteration.
								require.NoError(t, err)
								assert.Equal(t, "member2", clusterName)
							})
						}

					}

					// when
					clusterName, err := clusterBalancer.GetOptimalTargetCluster(ctx, capacity.OptimalTargetClusterFilter{})

					// then
					require.NoError(t, err)
					// expect that it would start provisioning in member2 again
					assert.Equal(t, "member2", clusterName)
				})
			}
		})
	}
}
