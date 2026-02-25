package metrics

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/version"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var logger = logf.Log.WithName("toolchain_metrics")

// counters
var (
	// UserSignupUniqueTotal is incremented only the first time a user signup is created, there is 1 for each unique user
	UserSignupUniqueTotal prometheus.Counter

	// UserSignupApprovedTotal is incremented each time a user signup is approved, can be multiple times per user if they reactivate multiple times
	UserSignupApprovedTotal prometheus.Counter

	// UserSignupApprovedWithMethodTotal is incremented each time a user signup is approved and includes either 'automatic' or 'manual' labels, can be multiple times per user if they reactivate multiple times
	UserSignupApprovedWithMethodTotal *prometheus.CounterVec

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

	// UserSignupVerificationRequiredTotal is incremented only the first time a user signup requires verification, can be multiple times per user if they reactivate multiple times
	UserSignupVerificationRequiredTotal prometheus.Counter
)

// gauge with labels
var (
	// SpaceGaugeVec reflects the current number of spaces in the system, with a label to partition per member cluster
	SpaceGaugeVec *prometheus.GaugeVec
	// UserSignupsPerActivationAndDomainGaugeVec reflects the number of users labelled with on their current number of activations and email address domain
	UserSignupsPerActivationAndDomainGaugeVec *prometheus.GaugeVec
	// MasterUserRecordGaugeVec reflects the current number of MasterUserRecords, labelled with their email address domain (`internal` vs `external`)
	MasterUserRecordGaugeVec *prometheus.GaugeVec
	// HostOperatorVersionGaugeVec reflects the current version of the host-operator (via the `version` label)
	HostOperatorVersionGaugeVec *prometheus.GaugeVec
	// ToolchainStatusGaugeVec reflects the current status of the toolchainstatus, labelled with the status
	ToolchainStatusGaugeVec *prometheus.GaugeVec
)

// histograms
var (
	// UserSignupProvisionTimeHistogram measures the provision time of the UserSignup needed either for its creation or reactivation
	UserSignupProvisionTimeHistogram prometheus.Histogram
	// UserSignupProvisionTimeHistogramBuckets is the list of buckets defined for UserSignupProvisionTimeHistogram
	UserSignupProvisionTimeHistogramBuckets = []float64{1, 2, 3, 5, 10, 30, 60, 120, 180, 360, 3600}
)

// collections
var (
	allCounters    = []prometheus.Counter{}
	allCounterVecs = []*prometheus.CounterVec{}
	allGauges      = []prometheus.Gauge{}
	allGaugeVecs   = []*prometheus.GaugeVec{}
	allHistograms  = []prometheus.Histogram{}
)

var cachedSpaceCounts = new(sync.Map)

func init() {
	initMetrics()
}

func initMetrics() {
	logger.Info("initializing custom metrics")
	// Counters
	UserSignupUniqueTotal = newCounter("user_signups_total", "Total number of unique UserSignups")
	UserSignupApprovedTotal = newCounter("user_signups_approved_total", "Total number of approved UserSignups")
	UserSignupApprovedWithMethodTotal = newCounterVec("user_signups_approved_with_method_total", "Total number of UserSignups approved, includes either 'automatic' or 'manual' labels for the approval method", "method")
	UserSignupBannedTotal = newCounter("user_signups_banned_total", "Total number of banned UserSignups")
	UserSignupDeactivatedTotal = newCounter("user_signups_deactivated_total", "Total number of deactivated UserSignups")
	UserSignupAutoDeactivatedTotal = newCounter("user_signups_auto_deactivated_total", "Total number of automatically deactivated UserSignups")
	UserSignupDeletedWithInitiatingVerificationTotal = newCounter("user_signups_deleted_with_initiating_verification_total", "Total number of UserSignups deleted after verification time trial and with verification initiated")
	UserSignupDeletedWithoutInitiatingVerificationTotal = newCounter("user_signups_deleted_without_initiating_verification_total", "Total number of deleted UserSignups after verification time trial but without verification initiated")
	UserSignupVerificationRequiredTotal = newCounter("user_signups_verification_required_total", "Total number of UserSignups that require verification, does not count verification attempts")
	// Gauges with labels
	SpaceGaugeVec = newGaugeVec("spaces_current", "Current number of Spaces (per member cluster)", "cluster_name")
	UserSignupsPerActivationAndDomainGaugeVec = newGaugeVec("users_per_activations_and_domain", "Number of UserSignups per activations and domain", []string{"activations", "domain"}...)
	MasterUserRecordGaugeVec = newGaugeVec("master_user_records", "Number of MasterUserRecords per email address domain ('internal' vs 'external')", "domain")
	HostOperatorVersionGaugeVec = newGaugeVec("host_operator_version", "Current version of the host operator", "commit")
	ToolchainStatusGaugeVec = newGaugeVec("toolchain_status", "Current status of the toolchain components", "component")
	// Histograms
	UserSignupProvisionTimeHistogram = newHistogram("user_signup_provision_time", "UserSignup provision time in seconds")
	logger.Info("custom metrics initialized")
}

const metricsPrefix = "sandbox_"

// Reset resets all  For testing purpose only!
func Reset() {
	logger.Info("resetting custom metrics and counts")
	for _, c := range allCounters {
		k8smetrics.Registry.Unregister(c)
	}
	for _, c := range allCounterVecs {
		k8smetrics.Registry.Unregister(c)
	}
	for _, g := range allGauges {
		k8smetrics.Registry.Unregister(g)
	}
	for _, v := range allGaugeVecs {
		k8smetrics.Registry.Unregister(v)
	}
	for _, v := range allHistograms {
		k8smetrics.Registry.Unregister(v)
	}
	initMetrics()
	cachedSpaceCounts.Clear()
	initialized = false // allow the counters to be initialized again
	logger.Info("metrics reset")
}

func newCounter(name, help string) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: metricsPrefix + name,
		Help: help,
	})
	allCounters = append(allCounters, c)
	return c
}

func newCounterVec(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricsPrefix + name,
		Help: help,
	}, labels)
	allCounterVecs = append(allCounterVecs, c)
	return c
}

func newGaugeVec(name, help string, labels ...string) *prometheus.GaugeVec {
	v := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricsPrefix + name,
		Help: help,
	}, labels)
	allGaugeVecs = append(allGaugeVecs, v)
	return v
}

func newHistogram(name, help string) prometheus.Histogram {
	v := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    metricsPrefix + name,
		Help:    help,
		Buckets: UserSignupProvisionTimeHistogramBuckets,
	})
	allHistograms = append(allHistograms, v)
	return v
}

// RegisterCustomMetrics registers the custom metrics
func RegisterCustomMetrics() []prometheus.Collector {
	collectors := []prometheus.Collector{}
	// register metrics
	for _, c := range allCounters {
		k8smetrics.Registry.MustRegister(c)
		collectors = append(collectors, c)
	}
	for _, c := range allCounterVecs {
		k8smetrics.Registry.MustRegister(c)
		collectors = append(collectors, c)
	}
	for _, g := range allGauges {
		k8smetrics.Registry.MustRegister(g)
		collectors = append(collectors, g)
	}
	for _, v := range allGaugeVecs {
		k8smetrics.Registry.MustRegister(v)
		collectors = append(collectors, v)
	}
	for _, v := range allHistograms {
		k8smetrics.Registry.MustRegister(v)
		collectors = append(collectors, v)
	}

	// expose the HostOperatorVersionGaugeVec metric (static ie, 1 value per build/deployment)
	HostOperatorVersionGaugeVec.WithLabelValues(version.Commit[0:7]).Set(1)

	logger.Info("custom metrics registered")
	return collectors
}

// IncrementMasterUserRecordCount increments the number of MasterUserRecord
func IncrementMasterUserRecordCount(logger logr.Logger, domain Domain) {
	logger.Info("incrementing MasterUserRecordGaugeVec", "domain", domain)
	MasterUserRecordGaugeVec.WithLabelValues(string(domain)).Inc()
}

// DecrementMasterUserRecordCount decreases the number of MasterUserRecord
func DecrementMasterUserRecordCount(logger logr.Logger, domain Domain) {
	logger.Info("decrementing MasterUserRecordGaugeVec", "domain", domain)
	MasterUserRecordGaugeVec.WithLabelValues(string(domain)).Dec()
}

// IncrementSpaceCount increments the number of Space's for the given member cluster
func IncrementSpaceCount(logger logr.Logger, clusterName string) {
	logger.Info("incrementing SpaceGaugeVec", "clusterName", clusterName)
	SpaceGaugeVec.WithLabelValues(clusterName).Inc()
	actual, _ := cachedSpaceCounts.LoadOrStore(clusterName, &atomic.Int32{})
	actual.(*atomic.Int32).Add(1)
	logger.Info("incremented SpaceGaugeVec", "clusterName", clusterName, "value", actual.(*atomic.Int32).Load())
}

// DecrementSpaceCount decreases the number of Spaces for the given member cluster
func DecrementSpaceCount(logger logr.Logger, clusterName string) {
	logger.Info("decrementing SpaceGaugeVec", "clusterName", clusterName)
	SpaceGaugeVec.WithLabelValues(clusterName).Dec()
	actual, _ := cachedSpaceCounts.LoadOrStore(clusterName, &atomic.Int32{})
	actual.(*atomic.Int32).Add(-1)
	logger.Info("decremented SpaceGaugeVec", "clusterName", clusterName, "value", actual.(*atomic.Int32).Load())
}

// IncrementUsersPerActivationCounters updates the activation counters and metrics
// When a user signs up for the 1st time, her `activations` number is `1`, on the second time, it's `2`, etc.
func IncrementUsersPerActivationCounters(logger logr.Logger, activations int, domain Domain) {
	logger.Info("incrementing UsersPerActivationAndDomainGaugeVec", "activations", activations, "domain", domain)
	// skip for invalid values
	if activations <= 0 {
		return
	}
	// increase the gauge with the given number of activations and the email address domain
	UserSignupsPerActivationAndDomainGaugeVec.WithLabelValues(strconv.Itoa(activations), string(domain)).Inc()
	// and decrease the gauge with the previous number of activations and the email address domain (if applicable)
	if activations > 1 {
		UserSignupsPerActivationAndDomainGaugeVec.WithLabelValues(strconv.Itoa(activations-1), string(domain)).Dec()
	}
}

// GetSpaceCountPerClusterSnapshot is a specialization of the GetCountsSnapshot function that retrieves just the map
// of the space counts per cluster.
func GetSpaceCountPerClusterSnapshot() map[string]int {
	spaceCounts := make(map[string]int)
	cachedSpaceCounts.Range(func(clusterName, count any) bool {
		spaceCounts[clusterName.(string)] = int(count.(*atomic.Int32).Load())
		return true
	})
	return spaceCounts
}

var initialized = false

// Synchronize synchronizes the content of the ToolchainStatus with the cached counter
//
// If the counter hasn't been initialized yet, then it adds the actual cached count
// to the one taken from ToolchainStatus and marks the cached as initialized
//
// If the ToolchainStatus doesn't contain any MUR counts, then it lists all existing
// MURs, counts them and stores in both cache and ToolchainStatus object
//
// If the cached counter is initialized and ToolchainStatus contains already some numbers
// then it updates the ToolchainStatus numbers with the one taken from the cached counter
func Synchronize(ctx context.Context, cl runtimeclient.Client, namespace string) error {
	// skip if cached counters are already initialized
	if initialized {
		logger.Info("skipping counters initialization because they are already initialized")
		return nil
	}

	logger.Info("initializing counters from resources")
	usersignups := &toolchainv1alpha1.UserSignupList{}
	if err := cl.List(ctx, usersignups, runtimeclient.InNamespace(namespace)); err != nil {
		return err
	}
	murs := &toolchainv1alpha1.MasterUserRecordList{}
	if err := cl.List(ctx, murs, runtimeclient.InNamespace(namespace)); err != nil {
		return err
	}
	spaces := &toolchainv1alpha1.SpaceList{}
	if err := cl.List(ctx, spaces, runtimeclient.InNamespace(namespace)); err != nil {
		return err
	}
	UserSignupsPerActivationAndDomainGaugeVec.Reset()
	for _, usersignup := range usersignups.Items {
		activations, activationsExists := usersignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]
		if activationsExists {
			_, err := strconv.Atoi(activations) // let's make sure the value is actually an integer
			if err != nil {
				logger.Error(err, "invalid number of activations", "name", usersignup.Name, "value", activations)
				continue
			}
			domain := GetEmailDomain(&usersignup) // nolint:gosec
			UserSignupsPerActivationAndDomainGaugeVec.WithLabelValues(activations, string(domain)).Inc()

		}
	}
	MasterUserRecordGaugeVec.Reset()
	for _, mur := range murs.Items {
		domain := GetEmailDomain(&mur) // nolint:gosec
		IncrementMasterUserRecordCount(logger, domain)
	}
	SpaceGaugeVec.Reset()
	for _, space := range spaces.Items {
		IncrementSpaceCount(logger, space.Spec.TargetCluster)
	}
	initialized = true
	logger.Info("metrics initialized from UserSignups, MasterUserRecords and Spaces", "userSignups", len(usersignups.Items), "murs", len(murs.Items), "spaces", len(spaces.Items))
	return nil
}
