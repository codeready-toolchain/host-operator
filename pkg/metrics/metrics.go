package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var log = logf.Log.WithName("toolchain_metrics")

// Counter is a wrapper of the prometheus counter type
type Counter struct {
	Name              string
	Help              string
	prometheusCounter prometheus.Counter
}

// Collector returns a collector for the metric
func (m Counter) Collector() prometheus.Collector {
	return m.prometheusCounter
}

// Increment the counter if it has been initialized
func (m Counter) Increment() {
	m.prometheusCounter.Inc()
}

// ConditionalIncrement increments the Counter if the condition is true
func (m Counter) ConditionalIncrement(condition bool) {
	if condition {
		m.prometheusCounter.Inc()
	}
}

// Gauge is a wrapper of the prometheus gauge type
type Gauge struct {
	Name            string
	Help            string
	prometheusGauge prometheus.Gauge
}

// Collector returns a collector for the metric
func (m Gauge) Collector() prometheus.Collector {
	return m.prometheusGauge
}

// Set the gauge if it has been initialized
func (m Gauge) Set(value float64) {
	m.prometheusGauge.Set(value)
}

// counters
var (
	// UserSignupUniqueTotal should be incremented only the first time a user signup is created, there should be 1 for each unique user
	UserSignupUniqueTotal = initCounter("user_signups_total", "Total number of unique User Signups")

	// UserSignupProvisionedTotal should be incremented each time a user signup is provisioned, can be multiple times per user if they reactivate multiple times
	UserSignupProvisionedTotal = initCounter("user_signups_provisioned_total", "Total number of Provisioned User Signups")

	// UserSignupBannedTotal should be incremented each time a user signup is banned
	UserSignupBannedTotal = initCounter("user_signups_banned_total", "Total number of Banned User Signups")

	// UserSignupDeactivatedTotal should be incremented each time a user signup is deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupDeactivatedTotal = initCounter("user_signups_deactivated_total", "Total number of Deactivated User Signups")

	// UserSignupAutoDeactivatedTotal should be incremented each time a user signup is automatically deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupAutoDeactivatedTotal = initCounter("user_signups_auto_deactivated_total", "Total number of Automatically Deactivated User Signups")
)

// gauges
var (
	// MasterUserRecordGauge should reflect the current number of master user records in the system
	MasterUserRecordGauge = initGauge("master_user_record_current", "Current number of Master User Records")
)

// collections
var (
	allCounters = []*Counter{}
	allGauges   = []*Gauge{}
)

func initCounter(name, help string) *Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: name,
		Help: help,
	})
	m := &Counter{
		Name:              name,
		Help:              help,
		prometheusCounter: c,
	}
	allCounters = append(allCounters, m)
	return m
}

func initGauge(name, help string) *Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	})
	m := &Gauge{
		Name:            name,
		Help:            help,
		prometheusGauge: g,
	}
	allGauges = append(allGauges, m)
	return m
}

// RegisterCustomMetrics registers the custom metrics
func RegisterCustomMetrics() {
	// register metrics
	for _, m := range allCounters {
		k8smetrics.Registry.MustRegister(m.prometheusCounter)
	}
	for _, m := range allGauges {
		k8smetrics.Registry.MustRegister(m.prometheusGauge)
	}
	log.Info("custom metrics registered successfully")
}

// UnregisterCustomMetrics unregisters the custom metrics
func UnregisterCustomMetrics() {
	for _, m := range allCounters {
		k8smetrics.Registry.Unregister(m.prometheusCounter)
	}
	for _, m := range allGauges {
		k8smetrics.Registry.Unregister(m.prometheusGauge)
	}
	log.Info("custom metrics unregistered successfully")
}

// Reset function for use in tests only
func Reset() {
	for _, m := range allCounters {
		m.prometheusCounter = prometheus.NewCounter(prometheus.CounterOpts{
			Name: m.Name,
			Help: m.Help,
		})
	}
	for _, m := range allGauges {
		m.prometheusGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: m.Name,
			Help: m.Help,
		})
	}
}
