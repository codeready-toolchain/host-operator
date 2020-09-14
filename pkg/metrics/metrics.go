package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var log = logf.Log.WithName("toolchain_metrics")

const (
	// UserSignupsGaugeName the name of the UserSignupsGauge counter
	UserSignupsGaugeName string = "user_signups"
)

var (
	// UserSignupsGauge counts the number of MasterUserRecords
	UserSignupsGauge prometheus.Gauge
)

// RegisterCustomMetrics registers the custom metrics
func RegisterCustomMetrics() {
	UserSignupsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: UserSignupsGaugeName,
		Help: "Number of User Signups",
	})
	k8smetrics.Registry.MustRegister(UserSignupsGauge)
	log.Info("custom metrics registered successfully")
}

// UnregisterCustomMetrics unregisters the custom metrics
func UnregisterCustomMetrics() {
	k8smetrics.Registry.Unregister(UserSignupsGauge)
	log.Info("custom metrics unregistered successfully")
}

// IncremementUserSignupGauge incremements the UserSignupsGauge
func IncremementUserSignupGauge() {
	if UserSignupsGauge == nil {
		log.Info("custom metrics not initialized")
		return
	}
	log.Info("incremented metric", "metric_name", UserSignupsGaugeName)
	UserSignupsGauge.Inc()
}
