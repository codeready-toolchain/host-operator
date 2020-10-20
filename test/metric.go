package test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/metrics"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func AssertMetricsCounterEquals(t *testing.T, expected int, counter *metrics.Counter) {
	assert.Equal(t, float64(expected), prom_testutil.ToFloat64(counter.Collector()))
}

func AssertMetricsGaugeEquals(t *testing.T, expected int, gauge *metrics.Gauge) {
	assert.Equal(t, float64(expected), prom_testutil.ToFloat64(gauge.Collector()))
}
