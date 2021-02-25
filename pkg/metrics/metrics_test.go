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

}

func TestResetMetrics(t *testing.T) {

	// when
	UserSignupUniqueTotal.Inc()
	MasterUserRecordGauge.Set(22)

	Reset()

	// then
	assert.Equal(t, float64(0), promtestutil.ToFloat64(UserSignupUniqueTotal))
	assert.Equal(t, float64(0), promtestutil.ToFloat64(MasterUserRecordGauge))
}
