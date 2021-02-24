package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var log = logf.Log.WithName("toolchain_metrics")

// counters
var (
	// UserSignupUniqueTotal should be incremented only the first time a user signup is created, there should be 1 for each unique user
	UserSignupUniqueTotal prometheus.Counter

	// UserSignupApprovedTotal should be incremented each time a user signup is approved, can be multiple times per user if they reactivate multiple times
	UserSignupApprovedTotal prometheus.Counter

	// UserSignupBannedTotal should be incremented each time a user signup is banned
	UserSignupBannedTotal prometheus.Counter

	// UserSignupDeactivatedTotal should be incremented each time a user signup is deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupDeactivatedTotal prometheus.Counter

	// UserSignupAutoDeactivatedTotal should be incremented each time a user signup is automatically deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupAutoDeactivatedTotal prometheus.Counter
)

// gauges
var (
	// MasterUserRecordGauge should reflect the current number of master user records in the system
	MasterUserRecordGauge prometheus.Gauge
)

// collections
var (
	allCounters = []prometheus.Counter{}
	allGauges   = []prometheus.Gauge{}
)

func init() {
	initMetrics()
}

const metricsPrefix = "sandbox_"

func initMetrics() {
	log.Info("initializing custom metrics")
	// Counters
	UserSignupUniqueTotal = newCounter("user_signups_total", "Total number of unique User Signups")
	UserSignupApprovedTotal = newCounter("user_signups_approved_total", "Total number of Approved User Signups")
	UserSignupBannedTotal = newCounter("user_signups_banned_total", "Total number of Banned User Signups")
	UserSignupDeactivatedTotal = newCounter("user_signups_deactivated_total", "Total number of Deactivated User Signups")
	UserSignupAutoDeactivatedTotal = newCounter("user_signups_auto_deactivated_total", "Total number of Automatically Deactivated User Signups")
	// Gauges
	MasterUserRecordGauge = newGauge("master_user_record_current", "Current number of Master User Records")
	log.Info("custom metrics initialized")
}

// Reset resets all metrics. For testing purpose only!
func Reset() {
	initMetrics()
}

func newCounter(name, help string) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: metricsPrefix + name,
		Help: help,
	})
	allCounters = append(allCounters, c)
	return c
}

func newGauge(name, help string) prometheus.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: metricsPrefix + name,
		Help: help,
	})
	allGauges = append(allGauges, g)
	return g
}

// RegisterCustomMetrics registers the custom metrics
func RegisterCustomMetrics() {
	// register metrics
	for _, c := range allCounters {
		k8smetrics.Registry.MustRegister(c)
	}
	for _, g := range allGauges {
		k8smetrics.Registry.MustRegister(g)
	}
	log.Info("custom metrics registered")
}
