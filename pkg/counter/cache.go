package counter

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("counter_cache")

var cachedCounts = cache{
	Counts: Counts{
		MasterUserRecordCount:        0,
		UserAccountsPerClusterCounts: map[string]int{},
		UsersPerActivationCounts:     map[string]int{},
	},
}

// Counts is type that contains number of MURs and number of UserAccounts per member cluster
type Counts struct {
	// Total number of Master User Accounts
	MasterUserRecordCount int
	// User Accounts per Clusters (indexed by cluster name)
	UserAccountsPerClusterCounts map[string]int
	// Activations per Users (number of users indexed by their number of activations)
	UsersPerActivationCounts map[string]int
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
		UserAccountsPerClusterCounts: map[string]int{},
		UsersPerActivationCounts:     map[string]int{},
	}
	cachedCounts.initialized = false
	metrics.MasterUserRecordGauge.Set(float64(0))
	metrics.UserAccountGaugeVec.Reset()
	metrics.UsersPerActivationGaugeVec.Reset()
}

// IncrementMasterUserRecordCount increments the number of MasterUserRecord in the cached counter
func IncrementMasterUserRecordCount() {
	write(func() {
		cachedCounts.MasterUserRecordCount++
		log.Info("incrementing MasterUserRecordGauge", "value", cachedCounts.MasterUserRecordCount)
		metrics.MasterUserRecordGauge.Set(float64(cachedCounts.MasterUserRecordCount))
	})
}

// DecrementMasterUserRecordCount decreases the number of MasterUserRecord in the cached counter
func DecrementMasterUserRecordCount(log logr.Logger) {
	write(func() {
		if cachedCounts.MasterUserRecordCount != 0 || !cachedCounts.initialized { // counter can be decreased even if its current value is `0`, but only if the cache has not been initialized yet
			cachedCounts.MasterUserRecordCount--
		} else {
			log.Error(fmt.Errorf("the count of MasterUserRecords is zero"),
				"unable to decrement the number of MasterUserRecords")
		}
		log.Info("decrementing MasterUserRecordGauge", "value", cachedCounts.MasterUserRecordCount)
		metrics.MasterUserRecordGauge.Set(float64(cachedCounts.MasterUserRecordCount))
	})
}

// IncrementUserAccountCount increments the number of UserAccount for the given member cluster in the cached counter
func IncrementUserAccountCount(clusterName string) {
	write(func() {
		cachedCounts.UserAccountsPerClusterCounts[clusterName]++
		metrics.UserAccountGaugeVec.WithLabelValues(clusterName).Set(float64(cachedCounts.UserAccountsPerClusterCounts[clusterName]))
	})
}

// DecrementUserAccountCount decreases the number of UserAccount for the given member cluster in the cached counter
func DecrementUserAccountCount(log logr.Logger, clusterName string) {
	write(func() {
		if cachedCounts.UserAccountsPerClusterCounts[clusterName] != 0 || !cachedCounts.initialized { // counter can be decreased even if its current value is `0`, but only if the cache has not been initialized yet
			cachedCounts.UserAccountsPerClusterCounts[clusterName]--
			metrics.UserAccountGaugeVec.WithLabelValues(clusterName).Set(float64(cachedCounts.UserAccountsPerClusterCounts[clusterName]))
		} else {
			log.Error(fmt.Errorf("the count of UserAccounts is zero"),
				"unable to decrement the number of UserAccounts for the given cluster", "cluster", clusterName)
		}
	})
}

// IncrementUsersPerActivationCounter increments the counter of a given number of activations for a (returning) user.
// When a user signs up for the 1st time, her `activations` number is `1`, on the second time, it's `2`, etc.
func IncrementUsersPerActivationCounter(activations int) {
	write(func() {
		// increase the gauge with the given number of activations
		cachedCounts.UsersPerActivationCounts[strconv.Itoa(activations)]++
		metrics.UsersPerActivationGaugeVec.WithLabelValues(strconv.Itoa(activations)).Inc()
		// and decrease the gauge with the previous number of activations (if applicable)
		if activations > 1 {
			cachedCounts.UsersPerActivationCounts[strconv.Itoa(activations-1)]--
			metrics.UsersPerActivationGaugeVec.WithLabelValues(strconv.Itoa(activations - 1)).Dec()
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
	toolchainStatus.Status.HostOperator.MasterUserRecordCount = cachedCounts.MasterUserRecordCount
	metrics.MasterUserRecordGauge.Set(float64(cachedCounts.MasterUserRecordCount))

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

	// update the toolchainStatus.Status.Metrics and metrics.UserAccountGaugeVec
	// from the cachedCounts.UsersPerActivationCounts
	if toolchainStatus.Status.Metrics == nil {
		toolchainStatus.Status.Metrics = map[string]toolchainv1alpha1.Metric{}
	}
	toolchainStatus.Status.Metrics[toolchainv1alpha1.UsersPerActivationMetricKey] = toolchainv1alpha1.Metric(cachedCounts.UsersPerActivationCounts)
	for activations, users := range cachedCounts.UsersPerActivationCounts {
		metrics.UsersPerActivationGaugeVec.WithLabelValues(activations).Set(float64(users))
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
	if toolchainStatus.Status.HostOperator == nil ||
		toolchainStatus.Status.HostOperator.MasterUserRecordCount == 0 {
		return initializeFromResources(cl, toolchainStatus.Namespace)
	}
	// otherwise, initialize the cached counters from the ToolchainStatud resource.
	return initializeFromToolchainStatus(cl, toolchainStatus)
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
		if value, exists := usersignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]; exists {
			_, err := strconv.Atoi(value) // let's make sure the value is actually an integer
			if err != nil {
				log.Error(err, "invalid number of activations", "name", usersignup.Name, "value", value)
				continue
			}
			cachedCounts.UsersPerActivationCounts[value]++
		} else {
			// annotation wasn't already set by the UserSignup controller at the time where the list was loaded,
			// so let's assume the value is '1'
			cachedCounts.UsersPerActivationCounts["1"]++
		}
	}
	for _, mur := range murs.Items {
		cachedCounts.MasterUserRecordCount++
		for _, ua := range mur.Spec.UserAccounts {
			cachedCounts.UserAccountsPerClusterCounts[ua.TargetCluster]++
		}
	}
	cachedCounts.initialized = true
	log.Info("cached counts initialized from UserSignups and MasterUserRecords",
		"MasterUserRecordCount", cachedCounts.MasterUserRecordCount,
		"UserAccountsPerClusterCounts", cachedCounts.UserAccountsPerClusterCounts,
		"UsersPerActivationCounts", cachedCounts.UsersPerActivationCounts)
	return nil
}

// initialize the cached counters from the ToolchainStatud resource.
func initializeFromToolchainStatus(cl client.Client, toolchainStatus *toolchainv1alpha1.ToolchainStatus) error {
	// initialize the cached counters from the ToolchainStatus resource
	if toolchainStatus.Status.HostOperator == nil {
		toolchainStatus.Status.HostOperator = &toolchainv1alpha1.HostOperatorStatus{}
	}
	// MasterUserRecordCount
	cachedCounts.MasterUserRecordCount += toolchainStatus.Status.HostOperator.MasterUserRecordCount
	// UserAccountsPerClusterCounts
	for _, memberStatus := range toolchainStatus.Status.Members {
		cachedCounts.UserAccountsPerClusterCounts[memberStatus.ClusterName] += memberStatus.UserAccountCount
	}
	// UsersPerActivationCounts
	if toolchainStatus.Status.Metrics != nil {
		if metric, exists := toolchainStatus.Status.Metrics[toolchainv1alpha1.UsersPerActivationMetricKey]; exists {
			cachedCounts.UsersPerActivationCounts = metric
		}
	}
	cachedCounts.initialized = true
	log.Info("cached counts initialized from ToolchainStatus")
	return nil
}
