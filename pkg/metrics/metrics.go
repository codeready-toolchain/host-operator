package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var log = logf.Log.WithName("toolchain_metrics")

// sandbox the namespace for all metrics
const sandbox = "sandbox"

// counters
var (
	// UserSignupUniqueTotal is incremented only the first time a user signup is created, i.e., 1 for each unique user
	UserSignupUniqueTotal = newCounter("user_signups_total", "Total number of unique User Signups")

	// UserSignupApprovedTotal should be incremented each time a user signup is approved, can be multiple times per user if they reactivate multiple times
	UserSignupApprovedTotal = newCounter("user_signups_approved_total", "Total number of Approved User Signups")

	// UserSignupBannedTotal is incremented each time a user signup is banned
	UserSignupBannedTotal = newCounter("user_signups_banned_total", "Total number of Banned User Signups")

	// UserSignupDeactivatedTotal is incremented each time a user signup is deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupDeactivatedTotal = newCounter("user_signups_deactivated_total", "Total number of Deactivated User Signups")

	// UserSignupAutoDeactivatedTotal is incremented each time a user signup is automatically deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupAutoDeactivatedTotal = newCounter("user_signups_auto_deactivated_total", "Total number of Automatically Deactivated User Signups")
)

// gauges
var (
	// MasterUserRecordGauge reflects the current number of master user records in the system
	MasterUserRecordGauge = newGauge("master_user_record_current", "Current number of Master User Records")

	// UserAccountsPerClusterGauge reflects the current number of user accounts per cluster
	UserAccountsPerClusterGauge = newGaugeVec("user_account_per_cluster_current", "Current number of UserAccounts per cluster", []string{ClusterNameLabel})

	OperatorInfoGauge = newGaugeVec("operator_info_current", "Info about the deployed operator", []string{BuildTimeLabel, CommitLabel, VersionLabel})
)

const (
	// ClusterNameLabel the gauge label to specify a cluster name
	ClusterNameLabel = "cluster_name"

	// BuildTimeLabel the gauge label to specify an operator build time
	BuildTimeLabel = "build_time"
	// CommitLabel the gauge label to specify an operator commit
	CommitLabel = "commit"
	// VersionLabel the gauge label to specify an operator version
	VersionLabel = "version"
)

// Reset resets the counters (for testing purpose only)
func Reset() {
	log.Info("resetting metrics")
	for _, m := range collectors {
		m.resetCollector()
	}
}

// RegisterCustomMetrics registers the custom metrics
func RegisterCustomMetrics() {
	// register metrics
	for _, m := range collectors {
		k8smetrics.Registry.MustRegister(m.Collector())
	}
	log.Info("custom metrics registered successfully")
}

// ------------------------------------------------
// Collectors (ie, the Prometheus metrics)
// ------------------------------------------------

// Collector the interface to access the underlying Prometheus metric
type Collector interface {
	Collector() prometheus.Collector
	resetCollector()
}

// all collectors (ie, metrics)
var (
	collectors = []Collector{}
)

// ------------------------------------------------
// Counters
// ------------------------------------------------

// Counter is a wrapper of the prometheus `Counter` type
type Counter struct {
	namespace string
	name      string
	help      string
	counter   prometheus.Counter
}

func newCounter(name, help string) *Counter {
	c := &Counter{
		name: name,
		help: help,
	}
	c.newCollector()
	collectors = append(collectors, c)
	return c
}

var _ Collector = &Counter{}

// Collector returns a collector for the metric
func (c *Counter) Collector() prometheus.Collector {
	return c.counter
}

// resetCollector reinitializes the Prometheus Collector (here, a Counter) for this wrapper
// Collector returns a collector for the metric
func (c *Counter) resetCollector() {
	c.newCollector()
}

// newCollector initializes a new Prometheus Counter for this wrapper
func (c *Counter) newCollector() {
	c.counter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: sandbox,
		Name:      c.name,
		Help:      c.help,
	})
}

// Increment the counter if it has been initialized
func (c *Counter) Increment() {
	c.counter.Inc()
}

// ConditionalIncrement increments the Counter if the condition is true
func (c *Counter) ConditionalIncrement(condition bool) {
	if condition {
		c.counter.Inc()
	}
}

// ------------------------------------------------
// Gauges
// ------------------------------------------------

// Gauge is a wrapper of the prometheus `Gauge` type
type Gauge struct {
	name  string
	help  string
	gauge prometheus.Gauge
}

// newGauge initializes a new Gauge wrapper
func newGauge(name, help string) *Gauge {
	g := &Gauge{
		name: name,
		help: help,
	}
	g.newCollector()
	collectors = append(collectors, g)
	return g
}

var _ Collector = &Gauge{}

// Collector returns a collector for the metric
func (g *Gauge) Collector() prometheus.Collector {
	return g.gauge
}

// resetCollector reinitializes the Prometheus Collector (here, a Gauge) for this wrapper
// Collector returns a collector for the metric
func (g *Gauge) resetCollector() {
	log.Info("resetting gauge collector", "name", g.name)
	g.newCollector()
}

// newCollector initializes a new Prometheus Gauge for this wrapper
func (g *Gauge) newCollector() {
	g.gauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: sandbox,
		Name:      g.name,
		Help:      g.help,
	})
}

// Set the gauge if it has been initialized
func (g *Gauge) Set(value float64) {
	log.Info("setting value on gauge", "name", g.name, "value", value)
	g.gauge.Set(value)
}

// GaugeVec is a wrapper of the prometheus `GaugeVec` type
type GaugeVec struct {
	name   string
	help   string
	labels []string
	gauge  *prometheus.GaugeVec
}

func newGaugeVec(name, help string, labels []string) *GaugeVec {
	g := &GaugeVec{
		name:   name,
		help:   help,
		labels: labels,
	}
	g.newCollector()
	collectors = append(collectors, g)
	return g
}

var _ Collector = &GaugeVec{}

// Collector returns a collector for the metric
func (g *GaugeVec) Collector() prometheus.Collector {
	return g.gauge
}

// resetCollector reinitializes the Prometheus Collector (here, a GaugeVec) for this wrapper
// Collector returns a collector for the metric
func (g *GaugeVec) resetCollector() {
	g.newCollector()
}

// newCollector initializes a new Prometheus Gauge for this wrapper
func (g *GaugeVec) newCollector() {
	g.gauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: sandbox,
		Name:      sandbox + g.name,
		Help:      g.help,
	}, g.labels)
}

// Set the gauge if it has been initialized
func (g *GaugeVec) Set(labels prometheus.Labels, value float64) error {
	m, err := g.gauge.GetMetricWith(labels)
	if err != nil {
		return err
	}
	m.Set(value)
	return nil
}

// // NewCollector returns a collector that exports metrics about current version
// // information.
// // see https://github.com/prometheus/common/blob/8558a5b7db3c84fa38b4766966059a7bd5bfa2ee/version/info.go#L36-L56
// func NewOperatorInfoCollector(program string) prometheus.Collector {
// 	return prometheus.NewGaugeFunc(
// 		prometheus.GaugeOpts{
// 			Namespace: sandbox,
// 			Name:      "build_info",
// 			Help: fmt.Sprintf(
// 				"A metric with a constant '1' value labeled by version, revision, branch, and goversion from which %s was built.",
// 				program,
// 			),
// 			ConstLabels: prometheus.Labels{
// 				"build_time",
// 				"commit",
// 				"version": Version,
// 			},
// 		},
// 		func() float64 { return 1 },
// 	)
// }
