package metrics

import (
	"testing"

	// . "github.com/codeready-toolchain/host-operator/test"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/stretchr/testify/assert"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestInitCounter(t *testing.T) {
	// given
	m := initCounter("test_counter", "test counter description")

	// when
	m.Increment()

	// then
	assert.Equal(t, "sandbox_test_counter", m.name)
	assert.Equal(t, "test counter description", m.help)
	assert.Equal(t, float64(1), prom_testutil.ToFloat64(m.prometheusCounter))
}

func TestInitGauge(t *testing.T) {
	// given
	m := initGauge("test_gauge", "test gauge description")

	// when
	m.Set(22)

	// then
	assert.Equal(t, "sandbox_test_gauge", m.name)
	assert.Equal(t, "test gauge description", m.help)
	assert.Equal(t, float64(22), prom_testutil.ToFloat64(m.prometheusGauge))
}

func TestRegisterCustomMetrics(t *testing.T) {
	// when
	RegisterCustomMetrics()

	// then
	// verify all metrics were registered successfully
	for _, m := range allCounters {
		assert.True(t, k8smetrics.Registry.Unregister(m.prometheusCounter))
	}

	for _, m := range allGauges {
		assert.True(t, k8smetrics.Registry.Unregister(m.prometheusGauge))
	}
}

func TestReset(t *testing.T) {
	// given
	c := initCounter("test_counter", "test counter description")
	g := initGauge("test_gauge", "test gauge description")

	// when
	c.Increment()
	g.Set(22)
	Reset()

	// then
	assert.Equal(t, float64(0), prom_testutil.ToFloat64(c.prometheusCounter))
	assert.Equal(t, float64(0), prom_testutil.ToFloat64(g.prometheusGauge))
}
