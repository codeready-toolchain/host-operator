package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var log = logf.Log.WithName("toolchain_metrics")

type counter struct {
	sync.RWMutex
	Name              string
	Help              string
	prometheusCounter prometheus.Counter
}

// counters
var (
	UserSignupTotal = &counter{
		Name: "user_signups_total",
		Help: "Number of User Signups",
	}

	UserSignupProvisionedTotal = &counter{
		Name: "user_signups_provisioned_total",
		Help: "Number of Provisioned User Signups",
	}

	UserSignupBannedTotal = &counter{
		Name: "user_signups_banned_total",
		Help: "Number of Banned User Signups",
	}

	UserSignupDeactivatedTotal = &counter{
		Name: "user_signups_deactivated_total",
		Help: "Number of Deactivated User Signups",
	}

	UserSignupAutoDeactivatedTotal = &counter{
		Name: "user_signups_auto_deactivated_total",
		Help: "Number of Automatically Deactivated User Signups",
	}
)

// collections
var (
	countersMap = make(map[string]*counter)
)

func initMetrics() {
	newCounter(UserSignupTotal, UserSignupProvisionedTotal, UserSignupBannedTotal, UserSignupDeactivatedTotal, UserSignupAutoDeactivatedTotal)
}

// RegisterCustomMetrics registers the custom metrics
func RegisterCustomMetrics() {
	initMetrics()
	// register metrics
	for _, ctr := range countersMap {
		k8smetrics.Registry.MustRegister(ctr.prometheusCounter)
	}
	log.Info("custom metrics registered successfully")
}

// UnregisterCustomMetrics unregisters the custom metrics
func UnregisterCustomMetrics() {
	for _, ctr := range countersMap {
		k8smetrics.Registry.Unregister(ctr.prometheusCounter)
	}
	log.Info("custom metrics unregistered successfully")
}

// IncrementUserSignupTotal increments the counter for total user signups
func IncrementUserSignupTotal() {
	incrementCounter(UserSignupTotal.Name)
}

// IncrementUserSignupProvisionedTotal increments the counter for total provisioned user signups
func IncrementUserSignupProvisionedTotal() {
	incrementCounter(UserSignupProvisionedTotal.Name)
}

// IncrementUserSignupBannedTotal increments the counter for total banned user signups
func IncrementUserSignupBannedTotal() {
	incrementCounter(UserSignupBannedTotal.Name)
}

// IncrementUserSignupDeactivatedTotal increments the counter for total deactivated user signups
func IncrementUserSignupDeactivatedTotal() {
	incrementCounter(UserSignupDeactivatedTotal.Name)
}

// IncrementUserSignupAutoDeactivatedTotal increments the counter for total automatically deactivated user signups
func IncrementUserSignupAutoDeactivatedTotal() {
	incrementCounter(UserSignupAutoDeactivatedTotal.Name)
}

func newCounter(metrics ...*counter) {
	for _, m := range metrics {
		ctr := prometheus.NewCounter(prometheus.CounterOpts{
			Name: m.Name,
			Help: m.Help,
		})
		m.prometheusCounter = ctr
		countersMap[m.Name] = m
	}
}

// IncrementCounter looks up the given counter and increments it
func incrementCounter(name string) {
	// check if counter was initialized
	counter, ok := countersMap[name]
	if !ok {
		log.Info("custom metric is not initialized", "name", name)
		return
	}
	// increment counter
	counter.Lock()
	defer counter.Unlock()
	counter.prometheusCounter.Inc()
}
