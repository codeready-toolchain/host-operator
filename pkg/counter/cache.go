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
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("counter_cache")

var cachedCounts = cache{
	Counts: Counts{
		SpacesPerClusterCounts:                  map[string]int{},
		UserSignupsPerActivationAndDomainCounts: map[string]int{},
		MasterUserRecordPerDomainCounts:         map[string]int{},
	},
}

// Counts is type that contains number of MURs and number of UserAccounts per member cluster
type Counts struct {
	// MasterUserRecordPerDomainCounts the number of MasterUserRecords per email address domain (`internal` vs `external`)
	MasterUserRecordPerDomainCounts map[string]int
	// SpacesPerClusterCounts the number of UserAccounts by cluster name
	SpacesPerClusterCounts map[string]int
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
		SpacesPerClusterCounts:                  map[string]int{},
		UserSignupsPerActivationAndDomainCounts: map[string]int{},
		MasterUserRecordPerDomainCounts:         map[string]int{},
	}
	cachedCounts.initialized = false
	metrics.SpaceGaugeVec.Reset()
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

// IncrementSpaceCount increments the number of Space's for the given member cluster in the cached counter
func IncrementSpaceCount(logger logr.Logger, clusterName string) {
	write(func() {
		cachedCounts.SpacesPerClusterCounts[clusterName]++
		logger.Info("incremented SpacesPerClusterCounts", "clusterName", clusterName, "value", cachedCounts.SpacesPerClusterCounts[clusterName])
		metrics.SpaceGaugeVec.WithLabelValues(clusterName).Set(float64(cachedCounts.SpacesPerClusterCounts[clusterName]))
	})
}

// DecrementSpaceCount decreases the number of Spaces for the given member cluster in the cached counter
func DecrementSpaceCount(logger logr.Logger, clusterName string) {
	write(func() {
		if cachedCounts.SpacesPerClusterCounts[clusterName] != 0 || !cachedCounts.initialized { // counter can be decreased even if its current value is `0`, but only if the cache has not been initialized yet
			cachedCounts.SpacesPerClusterCounts[clusterName]--
			metrics.SpaceGaugeVec.WithLabelValues(clusterName).Set(float64(cachedCounts.SpacesPerClusterCounts[clusterName]))
			logger.Info("decremented Spaces count", "clusterName", clusterName, "value", cachedCounts.SpacesPerClusterCounts[clusterName])
		} else {
			logger.Error(fmt.Errorf("the count of Spaces is zero"),
				"unable to decrement the number of Spaces for the given cluster", "cluster", clusterName)
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

// GetCountsSnapshot() returns Counts struct containing number of MURs and number of UserAccounts per member cluster as they were at the moment of the call.
// The returned struct is a copy of the cache and will not be updated concurrently.
// If the counter is not yet initialized, then it returns error
func GetCountsSnapshot() (Counts, error) {
	cachedCounts.RLock()
	defer cachedCounts.RUnlock()
	if !cachedCounts.initialized {
		return cachedCounts.Counts, fmt.Errorf("counter is not initialized")
	}
	counts := cachedCounts.Counts
	cpy := Counts{}
	cpy.MasterUserRecordPerDomainCounts = copyCountsMap(counts.MasterUserRecordPerDomainCounts)
	cpy.SpacesPerClusterCounts = copyCountsMap(counts.SpacesPerClusterCounts)
	cpy.UserSignupsPerActivationAndDomainCounts = copyCountsMap(counts.UserSignupsPerActivationAndDomainCounts)
	return cpy, nil
}

// GetSpaceCountPerClusterSnapshot is a specialization of the GetCountsSnapshot function that retrieves just the map
// of the space counts per cluster.
func GetSpaceCountPerClusterSnapshot() (map[string]int, error) {
	cachedCounts.RLock()
	defer cachedCounts.RUnlock()
	if !cachedCounts.initialized {
		return nil, fmt.Errorf("counter is not initialized")
	}
	return copyCountsMap(cachedCounts.SpacesPerClusterCounts), nil
}

func copyCountsMap(in map[string]int) map[string]int {
	cpy := make(map[string]int, len(in))
	for k, v := range in {
		cpy[k] = v
	}
	return cpy
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
func Synchronize(ctx context.Context, cl runtimeclient.Client, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	cachedCounts.Lock()
	defer cachedCounts.Unlock()

	// initialize the cached counters (if needed)
	if err := initialize(ctx, cl, toolchainStatus); err != nil {
		return err
	}

	log.Info("synchronizing counters", "cachedCounts.initialized", cachedCounts.initialized, "members", toolchainStatus.Status.Members)

	// update the toolchainStatus.HostOperator.MasterUserRecordCount and metrics.MasterUserRecordGauge
	// from the cachedCounts.MasterUserRecordCount
	if toolchainStatus.Status.HostOperator == nil {
		toolchainStatus.Status.HostOperator = &toolchainv1alpha1.HostOperatorStatus{}
	}

	// update the toolchainStatus.Status.Members and metrics.SpaceGaugeVec
	// from the cachedCounts.SpacesPerClusterCounts
	for _, member := range toolchainStatus.Status.Members {
		index := indexOfMember(toolchainStatus.Status.Members, member.ClusterName)
		if index == -1 {
			toolchainStatus.Status.Members = append(toolchainStatus.Status.Members, toolchainv1alpha1.Member{
				ClusterName: member.ClusterName,
			})
			index = len(toolchainStatus.Status.Members) - 1
		}

		spaceCount := cachedCounts.SpacesPerClusterCounts[member.ClusterName]
		toolchainStatus.Status.Members[index].SpaceCount = spaceCount
		log.Info("synchronized spaces_current gauge", "member_cluster", member.ClusterName, "count", spaceCount)
		metrics.SpaceGaugeVec.WithLabelValues(member.ClusterName).Set(float64(spaceCount))
	}

	// update the toolchainStatus.Status.Metrics and Prometheus metrics
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
		log.Info("synchronized master_user_records gauge", "domain", domain, "count", count)
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

func initialize(ctx context.Context, cl runtimeclient.Client, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// skip if cached counters are already initialized
	if cachedCounts.initialized {
		return nil
	}

	// initialize the cached counters from the UserSignup and MasterUserRecord resources.
	config, err := toolchainconfig.GetToolchainConfig(cl)
	if err != nil {
		return errors.Wrap(err, "unable to initialize counter cache")
	}
	_, masterUserRecordsPerDomainMetricExists := toolchainStatus.Status.Metrics[toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey]
	_, usersPerActivationAndDomainMetricKeyExists := toolchainStatus.Status.Metrics[toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey]
	if config.Metrics().ForceSynchronization() ||
		!usersPerActivationAndDomainMetricKeyExists ||
		!masterUserRecordsPerDomainMetricExists {
		return initializeFromResources(ctx, cl, toolchainStatus.Namespace)
	}
	// otherwise, initialize the cached counters from the ToolchainStatus resource.
	return initializeFromToolchainStatus(toolchainStatus)
}

// initialize the cached counters from the UserSignup and MasterUserRecord resources.
// this func lists all UserSignup and MasterUserRecord resources
func initializeFromResources(ctx context.Context, cl runtimeclient.Client, namespace string) error {
	log.Info("initializing counters from resources")
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
	reset()
	for _, usersignup := range usersignups.Items {
		activations, activationsExists := usersignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]
		if activationsExists {
			_, err := strconv.Atoi(activations) // let's make sure the value is actually an integer
			if err != nil {
				log.Error(err, "invalid number of activations", "name", usersignup.Name, "value", activations)
				continue
			}
			domain := metrics.GetEmailDomain(&usersignup) // nolint:gosec
			cachedCounts.UserSignupsPerActivationAndDomainCounts[joinLabelValues(activations, string(domain))]++
		}
	}
	for _, mur := range murs.Items {
		domain := metrics.GetEmailDomain(&mur) // nolint:gosec
		cachedCounts.MasterUserRecordPerDomainCounts[string(domain)]++
	}
	for _, space := range spaces.Items {
		cachedCounts.SpacesPerClusterCounts[space.Spec.TargetCluster]++
	}
	cachedCounts.initialized = true
	log.Info("cached counts initialized from UserSignups and MasterUserRecords",
		"MasterUserRecordPerDomainCounts", cachedCounts.MasterUserRecordPerDomainCounts,
		"UserSignupsPerActivationAndDomainCounts", cachedCounts.UserSignupsPerActivationAndDomainCounts,
		"SpacesPerClusterCounts", cachedCounts.SpacesPerClusterCounts,
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
	// SpacesPerClusterCounts
	for _, memberStatus := range toolchainStatus.Status.Members {
		cachedCounts.SpacesPerClusterCounts[memberStatus.ClusterName] += memberStatus.SpaceCount
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
