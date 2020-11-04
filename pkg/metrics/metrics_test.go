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
	m.Increment()

	// then
	assert.Equal(t, "sandbox_test_counter", m.name)
	assert.Equal(t, "test counter description", m.help)
	assert.Equal(t, float64(1), promtestutil.ToFloat64(m.counter))
}

func TestInitGauge(t *testing.T) {
	// given
	m := newGauge("test_gauge", "test gauge description")

	// when
	m.Set(22)

	// then
	assert.Equal(t, "sandbox_test_gauge", m.name)
	assert.Equal(t, "test gauge description", m.help)
	assert.Equal(t, float64(22), promtestutil.ToFloat64(m.gauge))
}

func TestRegisterCustomMetrics(t *testing.T) {
	// when
	RegisterCustomMetrics()

	// then
	// verify all metrics were registered successfully
	for _, m := range collectors {
		assert.True(t, k8smetrics.Registry.Unregister(m.Collector()))
	}
}

func TestResetCounters(t *testing.T) {
	// given
	c := newCounter("test_counter", "test counter description")
	c.Increment()
	g := newGauge("test_gauge", "test gauge description")
	g.Set(22)

	// when
	Reset()

	// then
	assert.Equal(t, float64(0), promtestutil.ToFloat64(c.counter))
	assert.Equal(t, float64(0), promtestutil.ToFloat64(g.gauge))
}
