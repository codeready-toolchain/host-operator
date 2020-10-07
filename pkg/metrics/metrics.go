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

// Increment the counter if it has been initialized
func (m Counter) Increment() {
	if m.prometheusCounter == nil {
		log.Info("metric has not been initialized", "name", m.Name)
		return
	}
	m.prometheusCounter.Inc()
}

// Gauge is a wrapper of the prometheus gauge type
type Gauge struct {
	Name            string
	Help            string
	prometheusGauge prometheus.Gauge
}

// Set the gauge if it has been initialized
func (m Gauge) Set(value float64) {
	if m.prometheusGauge == nil {
		log.Info("metric has not been initialized", "name", m.Name)
		return
	}
	m.prometheusGauge.Set(value)
}

// counters
var (
	// MasterUserRecordGauge should reflect the current number of master user records in the system
	MasterUserRecordGauge = &Gauge{
		Name: "master_user_record_current",
		Help: "Current number of Master User Records",
	}

	// UserSignupUniqueTotal should be incremented only the first time a user signup is created, there should be 1 for each unique user
	UserSignupUniqueTotal = &Counter{
		Name: "user_signups_total",
		Help: "Total number of unique User Signups",
	}

	// UserSignupProvisionedTotal should be incremented each time a user signup is provisioned, can be multiple times per user if they reactivate multiple times
	UserSignupProvisionedTotal = &Counter{
		Name: "user_signups_provisioned_total",
		Help: "Total number of Provisioned User Signups",
	}

	// UserSignupProvisionedTotal should be incremented each time a user signup is banned
	UserSignupBannedTotal = &Counter{
		Name: "user_signups_banned_total",
		Help: "Total number of Banned User Signups",
	}

	// UserSignupProvisionedTotal should be incremented each time a user signup is deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupDeactivatedTotal = &Counter{
		Name: "user_signups_deactivated_total",
		Help: "Total number of Deactivated User Signups",
	}

	// UserSignupAutoDeactivatedTotal should be incremented each time a user signup is automatically deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupAutoDeactivatedTotal = &Counter{
		Name: "user_signups_auto_deactivated_total",
		Help: "Total number of Automatically Deactivated User Signups",
	}
)

// collections
var (
	allCounters = []*Counter{}
	allGauges   = []*Gauge{}
)

func init() {
	initCounters(UserSignupUniqueTotal, UserSignupProvisionedTotal, UserSignupBannedTotal, UserSignupDeactivatedTotal, UserSignupAutoDeactivatedTotal)
	initGauges(MasterUserRecordGauge)
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

func initCounters(ctrs ...*Counter) {
	for _, m := range ctrs {
		c := prometheus.NewCounter(prometheus.CounterOpts{
			Name: m.Name,
			Help: m.Help,
		})
		m.prometheusCounter = c
		allCounters = append(allCounters, m)
	}
}

func initGauges(gauges ...*Gauge) {
	for _, m := range gauges {
		g := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: m.Name,
			Help: m.Help,
		})
		m.prometheusGauge = g
		allGauges = append(allGauges, m)
	}
}
