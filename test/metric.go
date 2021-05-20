package test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func AssertMetricsCounterEquals(t *testing.T, expected int, c prometheus.Counter) {
	assert.Equal(t, float64(expected), promtestutil.ToFloat64(c))
}

func AssertMetricsGaugeEquals(t *testing.T, expected int, g prometheus.Gauge, msgAndArgs ...interface{}) {
	assert.Equal(t, float64(expected), promtestutil.ToFloat64(g), msgAndArgs...)
}
