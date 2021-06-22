package counter

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("counter_cache")

var cachedCounts = cache{
	Counts: Counts{
		UserAccountsPerClusterCounts:            map[string]int{},
		UserSignupsPerActivationAndDomainCounts: map[string]int{},
		MasterUserRecordPerDomainCounts:         map[string]int{},
	},
}

// Counts is type that contains number of MURs and number of UserAccounts per member cluster
type Counts struct {
	// MasterUserRecordPerDomainCounts the number of MasterUserRecords per email address domain (`internal` vs `external`)
	MasterUserRecordPerDomainCounts map[string]int
	// UserAccountsPerClusterCounts the number of UserAccounts by cluster name
	UserAccountsPerClusterCounts map[string]int
	// UsersPerActivationCounts the number of users indexed by their number of activations and their email address domain. Eg: "1,internal","1,external",etc.
	UserSignupsPerActivationAndDomainCounts map[string]int
}

type cache struct {
	Counts
	sync.RWMutex
	initialized bool
}

func write(operation func()) {
	cachedCounts.Lock()
	defer cachedCounts.Unlock()
	operation()
}

// Reset resets the cached counter - is supposed to be used only in tests
func Reset() {
	write(func() {
		reset()
	})
}

func reset() {
	cachedCounts.Counts = Counts{
		UserAccountsPerClusterCounts:            map[string]int{},
		UserSignupsPerActivationAndDomainCounts: map[string]int{},
		MasterUserRecordPerDomainCounts:         map[string]int{},
	}
	cachedCounts.initialized = false
	metrics.UserAccountGaugeVec.Reset()
	metrics.UserSignupsPerActivationAndDomainGaugeVec.Reset()
	metrics.MasterUserRecordGaugeVec.Reset()
}

func (c Counts) MasterUserRecords() int {
	return c.MasterUserRecordPerDomainCounts[string(metrics.Internal)] + c.MasterUserRecordPerDomainCounts[string(metrics.External)]
}

// IncrementMasterUserRecordCount increments the number of MasterUserRecord in the cached counter
func IncrementMasterUserRecordCount(logger logr.Logger, domain metrics.Domain) {
	write(func() {
		cachedCounts.MasterUserRecordPerDomainCounts[string(domain)]++
		logger.Info("incrementing MasterUserRecordGaugeVec", "value", cachedCounts.MasterUserRecordPerDomainCounts[string(domain)], "domain", domain)
		metrics.MasterUserRecordGaugeVec.WithLabelValues(string(domain)).Set(float64(cachedCounts.MasterUserRecordPerDomainCounts[string(domain)]))
	})
}

// DecrementMasterUserRecordCount decreases the number of MasterUserRecord in the cached counter
func DecrementMasterUserRecordCount(logger logr.Logger, domain metrics.Domain) {
	write(func() {
		if cachedCounts.MasterUserRecordPerDomainCounts[string(domain)] != 0 || !cachedCounts.initialized { // counter can be decreased even if its current value is `0`, but only if the cache has not been initialized yet
			cachedCounts.MasterUserRecordPerDomainCounts[string(domain)]--
		} else {
			logger.Error(fmt.Errorf("the count of MasterUserRecords is zero"), "unable to decrement the number of MasterUserRecords", "domain", domain)
		}
		logger.Info("decrementing MasterUserRecordGaugeVec", "value", cachedCounts.MasterUserRecordPerDomainCounts[string(domain)], "domain", domain)
		metrics.MasterUserRecordGaugeVec.WithLabelValues(string(domain)).Set(float64(cachedCounts.MasterUserRecordPerDomainCounts[string(domain)]))
	})
}

// IncrementUserAccountCount increments the number of UserAccount for the given member cluster in the cached counter
func IncrementUserAccountCount(logger logr.Logger, clusterName string) {
	write(func() {
		cachedCounts.UserAccountsPerClusterCounts[clusterName]++
		logger.Info("incremented UserAccountsPerClusterCounts", "clusterName", clusterName, "value", cachedCounts.UserAccountsPerClusterCounts[clusterName])
		metrics.UserAccountGaugeVec.WithLabelValues(clusterName).Set(float64(cachedCounts.UserAccountsPerClusterCounts[clusterName]))
	})
}

// DecrementUserAccountCount decreases the number of UserAccount for the given member cluster in the cached counter
func DecrementUserAccountCount(logger logr.Logger, clusterName string) {
	write(func() {
		if cachedCounts.UserAccountsPerClusterCounts[clusterName] != 0 || !cachedCounts.initialized { // counter can be decreased even if its current value is `0`, but only if the cache has not been initialized yet
			cachedCounts.UserAccountsPerClusterCounts[clusterName]--
			metrics.UserAccountGaugeVec.WithLabelValues(clusterName).Set(float64(cachedCounts.UserAccountsPerClusterCounts[clusterName]))
			logger.Info("decremented UserAccount count", "value", cachedCounts.UserAccountsPerClusterCounts[clusterName])
		} else {
			logger.Error(fmt.Errorf("the count of UserAccounts is zero"),
				"unable to decrement the number of UserAccounts for the given cluster", "cluster", clusterName)
		}
	})
}

// UpdateUsersPerActivationCounters updates the activation counters and metrics
// When a user signs up for the 1st time, her `activations` number is `1`, on the second time, it's `2`, etc.
func UpdateUsersPerActivationCounters(logger logr.Logger, activations int, domain metrics.Domain) {
	write(func() {
		// skip for invalid values
		if activations <= 0 {
			return
		}
		// increase the gauge with the given number of activations and the email address domain
		cachedCounts.UserSignupsPerActivationAndDomainCounts[joinLabelValues(strconv.Itoa(activations), string(domain))]++
		metrics.UserSignupsPerActivationAndDomainGaugeVec.WithLabelValues(strconv.Itoa(activations), string(domain)).Inc()
		logger.Info("incremented UserSignupsPerActivationAndDomainCounts", "activations", activations, "domain", domain, "value", cachedCounts.UserSignupsPerActivationAndDomainCounts[joinLabelValues(strconv.Itoa(activations), string(domain))])
		// and decrease the gauge with the previous number of activations and the email address domain (if applicable)
		if activations > 1 {
			cachedCounts.UserSignupsPerActivationAndDomainCounts[joinLabelValues(strconv.Itoa(activations-1), string(domain))]--
			metrics.UserSignupsPerActivationAndDomainGaugeVec.WithLabelValues(strconv.Itoa(activations-1), string(domain)).Dec()
			logger.Info("decremented UserSignupsPerActivationAndDomainCounts", "activations", (activations - 1), "domain", domain, "value", cachedCounts.UserSignupsPerActivationAndDomainCounts[joinLabelValues(strconv.Itoa(activations-1), string(domain))])
		}
	})
}

// GetCounts returns Counts struct containing number of MURs and number of UserAccounts per member cluster.
// If the counter is not yet initialized, then it returns error
func GetCounts() (Counts, error) {
	cachedCounts.RLock()
	defer cachedCounts.RUnlock()
	if !cachedCounts.initialized {
		return cachedCounts.Counts, fmt.Errorf("counter is not initialized")
	}
	return cachedCounts.Counts, nil
}

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
func Synchronize(cl client.Client, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	cachedCounts.Lock()
	defer cachedCounts.Unlock()

	// initialize the cached counters (if needed)
	if err := initialize(cl, toolchainStatus); err != nil {
		return err
	}

	log.Info("synchronizing counters", "cachedCounts.initialized", cachedCounts.initialized, "members", toolchainStatus.Status.Members)

	// update the toolchainStatus.HostOperator.MasterUserRecordCount and metrics.MasterUserRecordGauge
	// from the cachedCounts.MasterUserRecordCount
	if toolchainStatus.Status.HostOperator == nil {
		toolchainStatus.Status.HostOperator = &toolchainv1alpha1.HostOperatorStatus{}
	}

	// update the toolchainStatus.Status.Members and metrics.UserAccountGaugeVec
	// from the cachedCounts.UserAccountsPerClusterCounts
	for _, member := range toolchainStatus.Status.Members {
		count := cachedCounts.UserAccountsPerClusterCounts[member.ClusterName]
		index := indexOfMember(toolchainStatus.Status.Members, member.ClusterName)
		if index == -1 {
			toolchainStatus.Status.Members = append(toolchainStatus.Status.Members, toolchainv1alpha1.Member{
				ClusterName: member.ClusterName,
			})
			index = len(toolchainStatus.Status.Members) - 1
		}
		toolchainStatus.Status.Members[index].UserAccountCount = count
		log.Info("synchronized user_accounts_current gauge", "member_cluster", member.ClusterName, "count", count)
		metrics.UserAccountGaugeVec.WithLabelValues(member.ClusterName).Set(float64(count))
	}

	// update the toolchainStatus.Status.Metrics and Pronetheus metrics
	// from the cachedCounts
	toolchainStatus.Status.Metrics = map[string]toolchainv1alpha1.Metric{} // reset, so any outdated/deprecated entry is automatically removed
	// `userSignupsPerActivationAndDomain` metric
	toolchainStatus.Status.Metrics[toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey] = toolchainv1alpha1.Metric(cachedCounts.UserSignupsPerActivationAndDomainCounts)
	for key, count := range cachedCounts.UserSignupsPerActivationAndDomainCounts {
		labels := splitLabelValues(key) // returns the values of the `activations` and `domain` labels
		metrics.UserSignupsPerActivationAndDomainGaugeVec.WithLabelValues(labels...).Set(float64(count))
	}
	// `masterUserRecordsPerDomain` metric
	toolchainStatus.Status.Metrics[toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey] = toolchainv1alpha1.Metric(cachedCounts.MasterUserRecordPerDomainCounts)
	for domain, count := range cachedCounts.MasterUserRecordPerDomainCounts {
		metrics.MasterUserRecordGaugeVec.WithLabelValues(domain).Set(float64(count))
	}
	log.Info("synchronized counters", "counts", cachedCounts.Counts)
	return nil
}

// retrieves the index of the member with the given name, or return -1 if not found
func indexOfMember(members []toolchainv1alpha1.Member, name string) int {
	for index, member := range members {
		if member.ClusterName == name {
			return index
		}
	}
	return -1
}

func initialize(cl client.Client, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// skip if cached counters are already initialized
	if cachedCounts.initialized {
		return nil
	}

	// initialize the cached counters from the UserSignup and MasterUserRecord resources.
	config, err := toolchainconfig.GetConfig(cl, toolchainStatus.Namespace)
	if err != nil {
		return errors.Wrap(err, "unable to initialize counter cache")
	}
	_, masterUserRecordsPerDomainMetricExists := toolchainStatus.Status.Metrics[toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey]
	_, usersPerActivationAndDomainMetricKeyExists := toolchainStatus.Status.Metrics[toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey]
	if config.Metrics().ForceSynchronization() ||
		!usersPerActivationAndDomainMetricKeyExists ||
		!masterUserRecordsPerDomainMetricExists {
		return initializeFromResources(cl, toolchainStatus.Namespace)
	}
	// otherwise, initialize the cached counters from the ToolchainStatud resource.
	return initializeFromToolchainStatus(toolchainStatus)
}

// initialize the cached counters from the UserSignup and MasterUserRecord resources.
// this func lists all UserSignup and MasterUserRecord resources
func initializeFromResources(cl client.Client, namespace string) error {
	log.Info("initializing counters from resources")
	usersignups := &toolchainv1alpha1.UserSignupList{}
	if err := cl.List(context.TODO(), usersignups, client.InNamespace(namespace)); err != nil {
		return err
	}
	murs := &toolchainv1alpha1.MasterUserRecordList{}
	if err := cl.List(context.TODO(), murs, client.InNamespace(namespace)); err != nil {
		return err
	}
	reset()
	for _, usersignup := range usersignups.Items {
		activations, activationsExists := usersignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]
		if activationsExists {
			_, err := strconv.Atoi(activations) // let's make sure the value is actually an integer
			if err != nil {
				log.Error(err, "invalid number of activations", "name", usersignup.Name, "value", activations)
				continue
			}
			domain := metrics.GetEmailDomain(&usersignup)
			cachedCounts.UserSignupsPerActivationAndDomainCounts[joinLabelValues(activations, string(domain))]++
		}
	}
	for _, mur := range murs.Items {
		domain := metrics.GetEmailDomain(&mur)
		cachedCounts.MasterUserRecordPerDomainCounts[string(domain)]++
		for _, ua := range mur.Spec.UserAccounts {
			cachedCounts.UserAccountsPerClusterCounts[ua.TargetCluster]++
		}
	}
	cachedCounts.initialized = true
	log.Info("cached counts initialized from UserSignups and MasterUserRecords",
		"MasterUserRecordPerDomainCounts", cachedCounts.MasterUserRecordPerDomainCounts,
		"UserAccountsPerClusterCounts", cachedCounts.UserAccountsPerClusterCounts,
		"UserSignupsPerActivationAndDomainCounts", cachedCounts.UserSignupsPerActivationAndDomainCounts,
	)
	return nil
}

// initialize the cached counters from the ToolchainStatud resource.
//
// Note: this func increments (NOT set!) the counters with the values stored in the ToolchainStatus,
// so that if user signups were processed before the init/sync started, we don't loose these numbers.
func initializeFromToolchainStatus(toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// initialize the cached counters from the ToolchainStatus resource
	if toolchainStatus.Status.HostOperator == nil {
		toolchainStatus.Status.HostOperator = &toolchainv1alpha1.HostOperatorStatus{}
	}

	// UserAccountsPerClusterCounts
	for _, memberStatus := range toolchainStatus.Status.Members {
		cachedCounts.UserAccountsPerClusterCounts[memberStatus.ClusterName] += memberStatus.UserAccountCount
	}
	if toolchainStatus.Status.Metrics != nil {
		// UserSignupsPerActivationAndDomainCounts
		if metric, exists := toolchainStatus.Status.Metrics[toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey]; exists {
			for k, v := range metric {
				cachedCounts.UserSignupsPerActivationAndDomainCounts[k] += v
			}
		}
		// MasterUserRecordCountPerDomain
		if metric, exists := toolchainStatus.Status.Metrics[toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey]; exists {
			for k, v := range metric {
				cachedCounts.MasterUserRecordPerDomainCounts[k] += v
			}
		}

	}
	cachedCounts.initialized = true
	log.Info("cached counts initialized from ToolchainStatus")
	return nil
}

func joinLabelValues(values ...string) string {
	return strings.Join(values, ",")
}

func splitLabelValues(values string) []string {
	return strings.Split(values, ",")
}
