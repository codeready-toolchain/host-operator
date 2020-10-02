package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var log = logf.Log.WithName("toolchain_metrics")

// counters
const (
	userSignupTotal                string = "user_signups_total"
	userSignupProvisionedTotal     string = "user_signups_provisioned_total"
	userSignupBannedTotal          string = "user_signups_banned_total"
	userSignupDeactivatedTotal     string = "user_signups_deactivated_total"
	userSignupAutoDeactivatedTotal string = "user_signups_auto_deactivated_total"
)

// userSignup counters
var (
	userSignupCounter, userSignupProvisionedCounter, userSignupBannedCounter, userSignupDeactivatedCounter, userSignupAutoDeactivatedCounter prometheus.Counter
)

// RegisterCustomMetrics registers the custom metrics
func RegisterCustomMetrics() {
	userSignupCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: userSignupTotal,
		Help: "Number of User Signups",
	})

	userSignupProvisionedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: userSignupProvisionedTotal,
		Help: "Number of Provisioned User Signups",
	})

	userSignupBannedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: userSignupBannedTotal,
		Help: "Number of Banned User Signups",
	})

	userSignupDeactivatedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: userSignupDeactivatedTotal,
		Help: "Number of Deactivated User Signups",
	})

	userSignupAutoDeactivatedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: userSignupAutoDeactivatedTotal,
		Help: "Number of Automatically Deactivated User Signups",
	})

	k8smetrics.Registry.MustRegister(userSignupCounter, userSignupProvisionedCounter, userSignupBannedCounter, userSignupDeactivatedCounter, userSignupAutoDeactivatedCounter)
	log.Info("custom metrics registered successfully")
}

// UnregisterCustomMetrics unregisters the custom metrics
func UnregisterCustomMetrics() {
	k8smetrics.Registry.Unregister(userSignupCounter)
	log.Info("custom metrics unregistered successfully")
}

// IncrementUserSignupCounter increments the userSignupCounter
func IncrementUserSignupCounter() {
	if userSignupCounter == nil {
		log.Info("custom metrics not initialized")
		return
	}
	log.Info("incremented metric", "metric_name", userSignupTotal)
	userSignupCounter.Inc()
}

// IncrementUserSignupProvisionedCounter increments the userSignupProvisionedCounter
func IncrementUserSignupProvisionedCounter() {
	if userSignupProvisionedCounter == nil {
		log.Info("custom metrics not initialized")
		return
	}
	log.Info("incremented metric", "metric_name", userSignupProvisionedTotal)
	userSignupProvisionedCounter.Inc()
}

// IncrementUserSignupBannedCounter increments the userSignupBannedCounter
func IncrementUserSignupBannedCounter() {
	if userSignupBannedCounter == nil {
		log.Info("custom metrics not initialized")
		return
	}
	log.Info("incremented metric", "metric_name", userSignupBannedTotal)
	userSignupBannedCounter.Inc()
}

// IncrementUserSignupDeactivatedCounter increments the userSignupDeactivatedCounter
func IncrementUserSignupDeactivatedCounter() {
	if userSignupDeactivatedCounter == nil {
		log.Info("custom metrics not initialized")
		return
	}
	log.Info("incremented metric", "metric_name", userSignupDeactivatedTotal)
	userSignupDeactivatedCounter.Inc()
}

// IncrementUserSignupAutoDeactivatedCounter increments the userSignupAutoDeactivatedCounter
func IncrementUserSignupAutoDeactivatedCounter() {
	if userSignupAutoDeactivatedCounter == nil {
		log.Info("custom metrics not initialized")
		return
	}
	log.Info("incremented metric", "metric_name", userSignupAutoDeactivatedTotal)
	userSignupAutoDeactivatedCounter.Inc()
}
