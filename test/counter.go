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

	initializeCounters(t, commontest.NewFakeClient(t, initObjs...), toolchainStatus)
}

func InitializeCountersWithoutReset(t *testing.T, toolchainStatus *toolchainv1alpha1.ToolchainStatus) {
	os.Setenv("WATCH_NAMESPACE", commontest.HostOperatorNs)
	t.Cleanup(counter.Reset)

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

	for _, count := range counts {
		options = append(options, WithMember(count.clusterName, WithSpaceCount(count.count)))
	}

	toolchainStatus := NewToolchainStatus(options...)

	toolchainConfig := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Metrics().ForceSynchronization(false))
	fakeClient := commontest.NewFakeClient(t, toolchainConfig)

	initializeCounters(t, fakeClient, toolchainStatus)
}

func initializeCounters(t *testing.T, cl *commontest.FakeClient, toolchainStatus *toolchainv1alpha1.ToolchainStatus) {
	t.Logf("toolchainStatus members: %v", toolchainStatus.Status.Members)
	err := counter.Synchronize(context.TODO(), cl, toolchainStatus)
	require.NoError(t, err)
}

func ClusterCount(clusterName string, count int) CountPerCluster {
	return CountPerCluster{clusterName: clusterName, count: count}
}
