package test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	metricscommon "github.com/codeready-toolchain/toolchain-common/pkg/test/metrics"
	spacetest "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	commonsignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type CounterAssertion struct {
	t      *testing.T
	counts counter.Counts
}

type CountPerCluster struct {
	clusterName string
	count       int
}

func AssertThatCountersAndMetrics(t *testing.T) *CounterAssertion {
	counts, err := counter.GetCountsSnapshot()
	require.NoError(t, err)
	return &CounterAssertion{
		t:      t,
		counts: counts,
	}
}

func AssertThatUninitializedCounters(t *testing.T) *CounterAssertion {
	counts, err := counter.GetCountsSnapshot()
	require.EqualErrorf(t, err, "counter is not initialized", "should be error because counter hasn't been initialized yet")
	return &CounterAssertion{
		t:      t,
		counts: counts,
	}
}

func (a *CounterAssertion) HaveSpacesForCluster(clusterName string, number int) *CounterAssertion {
	assert.Equal(a.t, number, a.counts.SpacesPerClusterCounts[clusterName])
	metricscommon.AssertMetricsGaugeEquals(a.t, number, metrics.SpaceGaugeVec.WithLabelValues(clusterName))
	return a
}

func (a *CounterAssertion) HaveUsersPerActivationsAndDomain(expected toolchainv1alpha1.Metric) *CounterAssertion {
	actual := a.counts.UserSignupsPerActivationAndDomainCounts
	assert.Equal(a.t, map[string]int(expected), actual)
	for key, count := range expected {
		metricscommon.AssertMetricsGaugeEquals(a.t, count, metrics.UserSignupsPerActivationAndDomainGaugeVec.WithLabelValues(strings.Split(key, ",")...))
	}
	return a
}

func (a *CounterAssertion) HaveMasterUserRecordsPerDomain(expected toolchainv1alpha1.Metric) *CounterAssertion {
	actual := a.counts.MasterUserRecordPerDomainCounts
	assert.Equal(a.t, map[string]int(expected), actual, "invalid counter values")
	for domain, count := range expected {
		metricscommon.AssertMetricsGaugeEquals(a.t, count, metrics.MasterUserRecordGaugeVec.WithLabelValues(domain), "invalid gauge value for domain '%v'", domain)
	}
	return a
}

func CreateMultipleMurs(t *testing.T, prefix string, number int, targetCluster string) []runtimeclient.Object {
	murs := make([]runtimeclient.Object, number)
	for index := range murs {
		murs[index] = masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("%s%d", prefix, index), masteruserrecord.TargetCluster(targetCluster))
	}
	return murs
}

func CreateMultipleUserSignups(prefix string, number int) []runtimeclient.Object {
	usersignups := make([]runtimeclient.Object, number)
	for index := range usersignups {
		usersignups[index] = commonsignup.NewUserSignup(
			commonsignup.WithName(fmt.Sprintf("%s%d", prefix, index)),
			commonsignup.WithAnnotation(toolchainv1alpha1.UserSignupActivationCounterAnnotationKey, strconv.Itoa(index+1)),
		)
	}
	return usersignups
}

func CreateMultipleSpaces(prefix string, number int, targetCluster string) []runtimeclient.Object {
	spaces := make([]runtimeclient.Object, number)
	for index := range spaces {
		spaces[index] = spacetest.NewSpace(commontest.HostOperatorNs, fmt.Sprintf("%s%d", prefix, index), spacetest.WithSpecTargetCluster(targetCluster))
	}
	return spaces
}

func InitializeCounters(t *testing.T, toolchainStatus *toolchainv1alpha1.ToolchainStatus, initObjs ...runtimeclient.Object) {
	os.Setenv("WATCH_NAMESPACE", commontest.HostOperatorNs)
	counter.Reset()
	t.Cleanup(counter.Reset)

	// Only create resources if none are provided (for ForceSynchronization=true scenarios)
	var allInitObjs []runtimeclient.Object
	allInitObjs = append(allInitObjs, initObjs...)

	// Only create additional resources if no test resources were provided
	if len(initObjs) == 0 {
		// Create Space resources based on member space counts
		for _, member := range toolchainStatus.Status.Members {
			for i := 0; i < member.SpaceCount; i++ {
				space := &toolchainv1alpha1.Space{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("space-%s-%d", member.ClusterName, i),
						Namespace: commontest.HostOperatorNs,
					},
					Spec: toolchainv1alpha1.SpaceSpec{
						TierName:      "base1ns",
						TargetCluster: member.ClusterName,
					},
				}
				allInitObjs = append(allInitObjs, space)
			}
		}

		// Create MasterUserRecord resources based on metrics
		if toolchainStatus.Status.Metrics != nil {
			if murMetrics, exists := toolchainStatus.Status.Metrics[toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey]; exists {
				murIndex := 0
				for domain, count := range murMetrics {
					for i := 0; i < count; i++ {
						mur := &toolchainv1alpha1.MasterUserRecord{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   commontest.HostOperatorNs,
								Name:        fmt.Sprintf("mur-%s-%d", domain, i),
								Labels:      map[string]string{},
								Annotations: map[string]string{},
							},
							Spec: toolchainv1alpha1.MasterUserRecordSpec{
								TierName: "deactivate30",
								PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
									Sub:         "UserID123",
									UserID:      "135246",
									AccountID:   "357468",
									OriginalSub: "11223344",
								},
							},
						}
						if domain == string(metrics.Internal) {
							mur.Spec.PropagatedClaims.Email = fmt.Sprintf("mur-%s-%d@redhat.com", domain, i)
						} else {
							mur.Spec.PropagatedClaims.Email = fmt.Sprintf("mur-%s-%d@external.com", domain, i)
						}
						allInitObjs = append(allInitObjs, mur)
						murIndex++
					}
				}
			}

			// Create UserSignup resources based on metrics
			if userSignupMetrics, exists := toolchainStatus.Status.Metrics[toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey]; exists {
				userIndex := 0
				for activationDomain, count := range userSignupMetrics {
					parts := strings.Split(activationDomain, ",")
					if len(parts) == 2 {
						activations := parts[0]
						domain := parts[1]
						for i := 0; i < count; i++ {
							userSignup := &toolchainv1alpha1.UserSignup{
								ObjectMeta: metav1.ObjectMeta{
									Name:      fmt.Sprintf("usersignup-%s-%s-%d", activations, domain, i),
									Namespace: commontest.HostOperatorNs,
									Annotations: map[string]string{
										toolchainv1alpha1.UserSignupActivationCounterAnnotationKey: activations,
									},
								},
								Spec: toolchainv1alpha1.UserSignupSpec{
									IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
										PreferredUsername: fmt.Sprintf("user-%d", userIndex),
									},
								},
							}
							// Set email to determine domain
							if domain == string(metrics.Internal) {
								userSignup.Spec.IdentityClaims.Email = fmt.Sprintf("user%d@redhat.com", userIndex)
							} else {
								userSignup.Spec.IdentityClaims.Email = fmt.Sprintf("user%d@external.com", userIndex)
							}
							allInitObjs = append(allInitObjs, userSignup)
							userIndex++
						}
					}
				}
			}
		}
	}

	initializeCounters(t, commontest.NewFakeClient(t, allInitObjs...), toolchainStatus)
}

func InitializeCountersWithoutReset(t *testing.T, toolchainStatus *toolchainv1alpha1.ToolchainStatus) {
	os.Setenv("WATCH_NAMESPACE", commontest.HostOperatorNs)
	t.Cleanup(func() {
		counter.Reset()
	})

	toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Metrics().ForceSynchronization(false))
	fakeClient := commontest.NewFakeClient(t, toolchainConfig)

	initializeCounters(t, fakeClient, toolchainStatus)
}

// InitializeCountersWith initializes the count cache from the counts parameter.
func InitializeCountersWith(t *testing.T, counts ...CountPerCluster) {
	os.Setenv("WATCH_NAMESPACE", commontest.HostOperatorNs)
	counter.Reset()
	t.Cleanup(counter.Reset)

	// we need the metrics to be present in the toolchain status so that we force the initialization of the counters from the toolchain status.
	// without the metrics, the counters would be intialized from MURs and user signups in the cluster (and because we're using a throw-away
	// fake client with no objects in it, that wouldn't work).
	options := []ToolchainStatusOption{
		WithMetric(toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey, toolchainv1alpha1.Metric{}),
		WithMetric(toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey, toolchainv1alpha1.Metric{}),
	}

	// Create actual Space resources for ForceSynchronization=true scenarios
	var initObjs []runtimeclient.Object
	for _, count := range counts {
		options = append(options, WithMember(count.clusterName, WithSpaceCount(count.count)))

		// Create Space resources that match the count
		for i := 0; i < count.count; i++ {
			space := &toolchainv1alpha1.Space{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("space-%s-%d", count.clusterName, i),
					Namespace: commontest.HostOperatorNs,
				},
				Spec: toolchainv1alpha1.SpaceSpec{
					TierName: "base1ns",

					TargetCluster: count.clusterName,
				},
			}
			initObjs = append(initObjs, space)
		}
	}

	toolchainStatus := NewToolchainStatus(options...)
	initializeCounters(t, commontest.NewFakeClient(t, initObjs...), toolchainStatus)
}

func initializeCounters(t *testing.T, cl *commontest.FakeClient, toolchainStatus *toolchainv1alpha1.ToolchainStatus) {
	t.Logf("toolchainStatus members: %v", toolchainStatus.Status.Members)
	err := counter.Synchronize(context.TODO(), cl, toolchainStatus)
	require.NoError(t, err)
}

func ClusterCount(clusterName string, count int) CountPerCluster {
	return CountPerCluster{clusterName: clusterName, count: count}
}
