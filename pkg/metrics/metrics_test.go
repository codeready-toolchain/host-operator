package metrics

import (
	"testing"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestInitCounter(t *testing.T) {
	// given
	m := newCounter("test_counter", "test counter description")

	// when
	m.Inc()

	// then
	assert.Equal(t, float64(1), promtestutil.ToFloat64(m))
}

func TestInitGauge(t *testing.T) {
	// given
	m := newGauge("test_gauge", "test gauge description")

	// when
	m.Set(22)

	// then
	assert.Equal(t, float64(22), promtestutil.ToFloat64(m))
}

func TestInitGaugeVec(t *testing.T) {
	// given
	m := newGaugeVec("test_gauge_vec", "test gauge description", "cluster_name")

	// when
	m.WithLabelValues("member-1").Set(1)
	m.WithLabelValues("member-2").Set(2)

	// then
	assert.Equal(t, float64(1), promtestutil.ToFloat64(m.WithLabelValues("member-1")))
	assert.Equal(t, float64(2), promtestutil.ToFloat64(m.WithLabelValues("member-2")))
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
	UserAccountGaugeVec.WithLabelValues("member-1").Set(20)

	Reset()

	// then
	assert.Equal(t, float64(0), promtestutil.ToFloat64(UserSignupUniqueTotal))
	assert.Equal(t, float64(0), promtestutil.ToFloat64(UserAccountGaugeVec.WithLabelValues("member-1")))
}
