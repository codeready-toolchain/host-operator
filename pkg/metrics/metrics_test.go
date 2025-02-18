package metrics

import (
	"testing"

	metricstest "github.com/codeready-toolchain/toolchain-common/pkg/test/metrics"
	"github.com/stretchr/testify/assert"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestInitCounter(t *testing.T) {
	// given
	m := newCounter("test_counter", "test counter description")

	// when
	m.Inc()

	// then
	metricstest.AssertMetricsCounterEquals(t, 1, m)
}

func TestInitGauge(t *testing.T) {
	// given
	m := newGauge("test_gauge", "test gauge description")

	// when
	m.Set(22)

	// then
	metricstest.AssertMetricsGaugeEquals(t, 22, m)
}

func TestInitGaugeVec(t *testing.T) {
	// given
	m := newGaugeVec("test_gauge_vec", "test gauge description", "cluster_name")

	// when
	m.WithLabelValues("member-1").Set(1)
	m.WithLabelValues("member-2").Set(2)

	// then
	metricstest.AssertMetricsGaugeEquals(t, 1, m.WithLabelValues("member-1"))
	metricstest.AssertMetricsGaugeEquals(t, 2, m.WithLabelValues("member-2"))
}

func TestRegisterCustomMetrics(t *testing.T) {
	// when
	RegisterCustomMetrics()

	// then
	// verify all metrics were registered successfully
	for _, m := range allCounters {
		assert.True(t, k8smetrics.Registry.Unregister(m))
	}

	for _, m := range allGauges {
		assert.True(t, k8smetrics.Registry.Unregister(m))
	}

	for _, m := range allGaugeVecs {
		assert.True(t, k8smetrics.Registry.Unregister(m))
	}
}

func TestResetMetrics(t *testing.T) {
	// when
	UserSignupUniqueTotal.Inc()
	SpaceGaugeVec.WithLabelValues("member-1").Set(20)

	Reset()

	// then
	metricstest.AssertMetricsCounterEquals(t, 0, UserSignupUniqueTotal)
	metricstest.AssertMetricsGaugeEquals(t, 0, SpaceGaugeVec.WithLabelValues("member-1"))
}
