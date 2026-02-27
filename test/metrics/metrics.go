package metrics

import (
	"context"
	"os"
	"strings"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	metricscommontest "github.com/codeready-toolchain/toolchain-common/pkg/test/metrics"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MetricAssertion struct {
	t           *testing.T
	spaceCounts map[string]int
}

func AssertThatCountersAndMetrics(t *testing.T) *MetricAssertion {
	spaceCounts := metrics.GetSpaceCountPerClusterSnapshot()
	return &MetricAssertion{
		t:           t,
		spaceCounts: spaceCounts, //nolint:staticcheck
	}
}

func (a *MetricAssertion) HaveSpacesForCluster(clusterName string, number int) *MetricAssertion {
	assert.Equal(a.t, number, a.spaceCounts[clusterName])
	metricscommontest.AssertMetricsGaugeEquals(a.t, number, metrics.SpaceGaugeVec.WithLabelValues(clusterName))
	return a
}

func (a *MetricAssertion) HaveUsersPerActivationsAndDomain(expected toolchainv1alpha1.Metric) *MetricAssertion {
	for key, count := range expected {
		metricscommontest.AssertMetricsGaugeEquals(a.t, count, metrics.UserSignupsPerActivationAndDomainGaugeVec.WithLabelValues(strings.Split(key, ",")...), "invalid gauge value for key '%v'", key)
	}
	return a
}

func (a *MetricAssertion) HaveMasterUserRecordsPerDomain(expected toolchainv1alpha1.Metric) *MetricAssertion {
	for domain, count := range expected {
		metricscommontest.AssertMetricsGaugeEquals(a.t, count, metrics.MasterUserRecordGaugeVec.WithLabelValues(domain), "invalid gauge value for domain '%v'", domain)
	}
	return a
}

func ResetCounters(t *testing.T, initObjs ...runtimeclient.Object) {
	os.Setenv("WATCH_NAMESPACE", commontest.HostOperatorNs)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	cl := commontest.NewFakeClient(t, initObjs...)
	err := metrics.Synchronize(context.TODO(), cl, commontest.HostOperatorNs)
	require.NoError(t, err)
}
