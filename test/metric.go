package test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func AssertMetricsCounterEquals(t *testing.T, expected int, counter prometheus.Counter) {
	assert.Equal(t, float64(expected), promtestutil.ToFloat64(counter))
}

func AssertMetricsGaugeEquals(t *testing.T, expected int, gauge prometheus.Gauge) {
	assert.Equal(t, float64(expected), promtestutil.ToFloat64(gauge))
}
