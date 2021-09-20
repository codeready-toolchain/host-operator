package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var log = logf.Log.WithName("toolchain_metrics")

// counters
var (
	// UserSignupUniqueTotal is incremented only the first time a user signup is created, there is 1 for each unique user
	UserSignupUniqueTotal prometheus.Counter

	// UserSignupApprovedTotal is incremented each time a user signup is approved, can be multiple times per user if they reactivate multiple times
	UserSignupApprovedTotal prometheus.Counter

	// UserSignupBannedTotal is incremented each time a user signup is banned
	UserSignupBannedTotal prometheus.Counter

	// UserSignupDeactivatedTotal is incremented each time a user signup is deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupDeactivatedTotal prometheus.Counter

	// UserSignupAutoDeactivatedTotal is incremented each time a user signup is automatically deactivated, can be multiple times per user if they reactivate multiple times
	UserSignupAutoDeactivatedTotal prometheus.Counter

	// UserSignupDeletedWithInitiatingVerificationTotal is incremented each time a user signup is deleted due to verification time trial expired, and verification was initiated
	UserSignupDeletedWithInitiatingVerificationTotal prometheus.Counter

	// UserSignupDeletedWithoutInitiatingVerificationTotal is incremented each time a user signup is deleted due to verification time trial expired, and verification was NOT initiated
	UserSignupDeletedWithoutInitiatingVerificationTotal prometheus.Counter
)

// gauge with labels
var (
	// UserAccountGaugeVec reflects the current number of master user records in the system, with a label to partition per member cluster
	UserAccountGaugeVec *prometheus.GaugeVec
	// UserSignupsPerActivationAndDomainGaugeVec reflects the number of users labelled with on their current number of activations and email address domain
	UserSignupsPerActivationAndDomainGaugeVec *prometheus.GaugeVec
	// MasterUserRecordGaugeVec reflects the current number of MasterUserRecords, labelled with their email address domain (`internal` vs `external`)
	MasterUserRecordGaugeVec *prometheus.GaugeVec
)

// collections
var (
	allCounters  = []prometheus.Counter{}
	allGauges    = []prometheus.Gauge{}
	allGaugeVecs = []*prometheus.GaugeVec{}
)

func init() {
	initMetrics()
}

const metricsPrefix = "sandbox_"

func initMetrics() {
	log.Info("initializing custom metrics")
	// Counters
	UserSignupUniqueTotal = newCounter("user_signups_total", "Total number of unique UserSignups")
	UserSignupApprovedTotal = newCounter("user_signups_approved_total", "Total number of approved UserSignups")
	UserSignupBannedTotal = newCounter("user_signups_banned_total", "Total number of banned UserSignups")
	UserSignupDeactivatedTotal = newCounter("user_signups_deactivated_total", "Total number of deactivated UserSignups")
	UserSignupAutoDeactivatedTotal = newCounter("user_signups_auto_deactivated_total", "Total number of automatically deactivated UserSignups")
	UserSignupDeletedWithInitiatingVerificationTotal = newCounter("user_signups_deleted_with_initiating_verification_total", "Total number of UserSignups deleted after verification time trial and with verification initiated")
	UserSignupDeletedWithoutInitiatingVerificationTotal = newCounter("user_signups_deleted_without_initiating_verification_total", "Total number of deleted UserSignups after verification time trial but without verification initiated")
	// Gauges with labels
	UserAccountGaugeVec = newGaugeVec("user_accounts_current", "Current number of UserAccounts (per member cluster)", "cluster_name")
	UserSignupsPerActivationAndDomainGaugeVec = newGaugeVec("users_per_activations_and_domain", "Number of UserSignups per activations and domain", []string{"activations", "domain"}...)
	MasterUserRecordGaugeVec = newGaugeVec("master_user_records", "Number of MasterUserRecords per email address domain ('internal' vs 'external')", "domain")
	log.Info("custom metrics initialized")
}

// Reset resets all metrics. For testing purpose only!
func Reset() {
	log.Info("resetting custom metrics")
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

func newGaugeVec(name, help string, labels ...string) *prometheus.GaugeVec {
	v := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricsPrefix + name,
		Help: help,
	}, labels)
	allGaugeVecs = append(allGaugeVecs, v)
	return v
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
	for _, v := range allGaugeVecs {
		k8smetrics.Registry.MustRegister(v)
	}
	log.Info("custom metrics registered")
}
