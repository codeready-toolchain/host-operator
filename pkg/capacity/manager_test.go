package capacity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	metricstest "github.com/codeready-toolchain/host-operator/test/metrics"
	hspc "github.com/codeready-toolchain/host-operator/test/spaceprovisionerconfig"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
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
	for preferred := range 3 {
		member1s := generateMemberConfigCombinations(commontest.MemberClusterName, preferred == 1)
		member2s := generateMemberConfigCombinations(commontest.Member2ClusterName, preferred == 2)

		for _, member1 := range member1s {
			for _, member2 := range member2s {
				testCases = append(testCases, getOptimalTargetClusterTestCase{member1: member1, member2: member2})
			}
		}
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("selecting between member1(%+v) and member2(%+v) should yield %s", tc.member1, tc.member2, tc.getSelectedMemberName()), func(t *testing.T) {
			// given
			initObjs := []runtimeclient.Object{
				tc.member1.buildSpaceProvisionerConfig(),
				tc.member2.buildSpaceProvisionerConfig(),
			}
			for i := range 700 {
				initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("john-space-%d", i), spacetest.WithStatusTargetCluster(tc.member1.clusterName)))
			}
			for i := range 500 {
				initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("jack-space-%d", i), spacetest.WithStatusTargetCluster(tc.member2.clusterName)))
			}
			fakeClient := commontest.NewFakeClient(t, initObjs...)
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
		cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

		// when
		clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{})

		// then
		require.NoError(t, err)
		assert.Equal(t, "member1", clusterName)
	})

	t.Run("with three eligible clusters it returns the one with the most capacity (the first one)", func(t *testing.T) {
		// given
		initObjs := []runtimeclient.Object{
			hspc.NewEnabledValidTenantSPC(commontest.MemberClusterName, spc.MaxNumberOfSpaces(10000)),
			hspc.NewEnabledValidTenantSPC(commontest.Member2ClusterName, spc.MaxNumberOfSpaces(1000)),
			hspc.NewEnabledValidTenantSPC("member3-cluster", spc.MaxNumberOfSpaces(1000)),
		}
		for i := range 700 {
			initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("john-space-%d", i), spacetest.WithSpecTargetCluster(commontest.MemberClusterName)))
		}
		for i := range 700 {
			initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("jack-space-%d", i), spacetest.WithSpecTargetCluster(commontest.Member2ClusterName)))
		}
		for i := range 200 {
			initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("jeff-space-%d", i), spacetest.WithSpecTargetCluster("member3-cluster")))
		}
		metricstest.ResetCounters(t, initObjs...)
		fakeClient := commontest.NewFakeClient(t, initObjs...)
		cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

		// when
		clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{})

		// then
		require.NoError(t, err)
		assert.Equal(t, commontest.MemberClusterName, clusterName)
	})

	t.Run("with three eligible clusters it returns the one with the most capacity (the third one)", func(t *testing.T) {
		// given
		initObjs := []runtimeclient.Object{
			hspc.NewEnabledValidTenantSPC(commontest.MemberClusterName, spc.MaxNumberOfSpaces(1000)),
			hspc.NewEnabledValidTenantSPC(commontest.Member2ClusterName, spc.MaxNumberOfSpaces(1000)),
			hspc.NewEnabledValidTenantSPC("member3-cluster", spc.MaxNumberOfSpaces(1000)),
		}
		for i := range 700 {
			initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("john-space-%d", i), spacetest.WithSpecTargetCluster(commontest.MemberClusterName)))
		}
		for i := range 700 {
			initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("jack-space-%d", i), spacetest.WithSpecTargetCluster(commontest.Member2ClusterName)))
		}
		for i := range 200 {
			initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("jeff-space-%d", i), spacetest.WithSpecTargetCluster("member3-cluster")))
		}
		metricstest.ResetCounters(t, initObjs...)
		fakeClient := commontest.NewFakeClient(t, initObjs...)
		cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

		// when
		clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{})

		// then
		require.NoError(t, err)
		assert.Equal(t, "member3-cluster", clusterName)
	})

	t.Run("choose one of the configured clusters because the preferred one is missing the SPC", func(t *testing.T) {
		// given
		initObjs := []runtimeclient.Object{
			hspc.NewEnabledValidTenantSPC(commontest.MemberClusterName, spc.MaxNumberOfSpaces(1000)),
			hspc.NewEnabledValidTenantSPC(commontest.Member2ClusterName, spc.MaxNumberOfSpaces(1000)),
		}
		for i := range 700 {
			initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("john-space-%d", i), spacetest.WithSpecTargetCluster(commontest.MemberClusterName)))
		}
		for i := range 500 {
			initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("jack-space-%d", i), spacetest.WithSpecTargetCluster(commontest.Member2ClusterName)))
		}
		metricstest.ResetCounters(t, initObjs...)
		fakeClient := commontest.NewFakeClient(t, initObjs...)
		cm := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

		// when
		clusterName, err := cm.GetOptimalTargetCluster(context.TODO(), capacity.OptimalTargetClusterFilter{
			PreferredCluster: "member3-cluster",                     // request specifically this member eve if it doesn't match the cluster-roles from below
			ClusterRoles:     []string{spc.PlacementRole("tenant")}, // set
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, commontest.Member2ClusterName, clusterName)
	})

	t.Run("failures", func(t *testing.T) {
		t.Run("unable to list SpaceProvisionerConfigs", func(t *testing.T) {
			// given
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
					initObjs := []runtimeclient.Object{
						hspc.NewEnabledValidTenantSPC(commontest.MemberClusterName, spc.MaxNumberOfSpaces(1000)),
						hspc.NewEnabledValidTenantSPC(commontest.Member2ClusterName, spc.MaxNumberOfSpaces(limit)),
						hspc.NewEnabledValidTenantSPC("member3-cluster", spc.MaxNumberOfSpaces(limit)),
					}
					for i := range 1000 {
						initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("john-space-%d", i), spacetest.WithSpecTargetCluster(commontest.MemberClusterName)))
					}
					for i := range numberOfSpaces {
						initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("jack-space-%d", i), spacetest.WithSpecTargetCluster(commontest.Member2ClusterName)))
						initObjs = append(initObjs, spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("jeff-space-%d", i), spacetest.WithSpecTargetCluster("member3-cluster")))
					}
					metricstest.ResetCounters(t, initObjs...)
					fakeClient := commontest.NewFakeClient(t, initObjs...)
					clusterBalancer := capacity.NewClusterManager(commontest.HostOperatorNs, fakeClient)

					// now run in 4 cycles and expect that the users will be provisioned in batches of 50
					member2CurrentCount := numberOfSpaces
					member3CurrentCount := numberOfSpaces
					for cycle := range 4 {

						member2MissingTo50 := 50 - member2CurrentCount%50
						// this 50 users should go into member2 - it will be always 50
						for i := range member2MissingTo50 {
							t.Run(fmt.Sprintf("cycle %d user %d for member2", cycle, i), func(t *testing.T) {
								// given
								// even when the counter of the other member is decremented, it should still use the last used one
								// but we can decrement it only in the second cycle when the member3 has at least 50 Spaces
								if i == 2 && cycle > 1 {
									metrics.DecrementSpaceCount(log.Log, "member3-cluster")
									member3CurrentCount--
								}

								// when
								clusterName, err := clusterBalancer.GetOptimalTargetCluster(ctx, capacity.OptimalTargetClusterFilter{})

								// then
								require.NoError(t, err)
								assert.Equal(t, commontest.Member2ClusterName, clusterName)

								metrics.IncrementSpaceCount(log.Log, commontest.Member2ClusterName)
								member2CurrentCount++

								// and when calling it with the other cluster as preferred
								clusterName, err = clusterBalancer.GetOptimalTargetCluster(ctx, capacity.OptimalTargetClusterFilter{
									PreferredCluster: "member3-cluster",
								})

								// then it should return the preferred one, but it shouldn't have any effect on the "balancing and batching" logic in the following iteration.
								require.NoError(t, err)
								assert.Equal(t, "member3-cluster", clusterName)
							})
						}

						// reset the decremented counter back
						if member2MissingTo50 > 2 && cycle > 1 {
							metrics.IncrementSpaceCount(log.Log, "member3-cluster")
							member3CurrentCount++
						}

						member3MissingTo50 := 50 - member3CurrentCount%50
						// this batch of users should go into member3 - the size of the batch depends on how many times the cluster is bigger than member2
						for i := range member3MissingTo50 {
							t.Run(fmt.Sprintf("cycle %d user %d for member3-cluster", cycle, i), func(t *testing.T) {
								// when
								clusterName, err := clusterBalancer.GetOptimalTargetCluster(
									ctx,
									capacity.OptimalTargetClusterFilter{},
								)

								// then
								require.NoError(t, err)
								assert.Equal(t, "member3-cluster", clusterName)

								metrics.IncrementSpaceCount(log.Log, "member3-cluster")
								member3CurrentCount++

								// and when calling it with the other cluster as preferred
								clusterName, err = clusterBalancer.GetOptimalTargetCluster(ctx, capacity.OptimalTargetClusterFilter{
									PreferredCluster: commontest.Member2ClusterName,
								})

								// then it should return the preferred one, but it shouldn't have any effect on the "balancing and batching" logic in the following iteration.
								require.NoError(t, err)
								assert.Equal(t, commontest.Member2ClusterName, clusterName)
							})
						}

					}

					// when
					clusterName, err := clusterBalancer.GetOptimalTargetCluster(ctx, capacity.OptimalTargetClusterFilter{})

					// then
					require.NoError(t, err)
					// expect that it would start provisioning in member2 again
					assert.Equal(t, commontest.Member2ClusterName, clusterName)
				})
			}
		})
	}
}
