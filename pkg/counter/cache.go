package counter

import (
	"context"
	"fmt"
	"sync"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var cachedCounts = cache{
	Counts: Counts{
		UserAccountsPerClusterCounts: map[string]int{},
	},
}

// Counts is type that contains number of MURs and number of UserAccounts per member cluster
type Counts struct {
	MasterUserRecordCount        int
	UserAccountsPerClusterCounts map[string]int
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
	}
	cachedCounts.initialized = false
}

// IncrementMasterUserRecordCount increments the number of MasterUserRecord in the cached counter
func IncrementMasterUserRecordCount() {
	write(func() {
		cachedCounts.MasterUserRecordCount++
		metrics.MasterUserRecordGauge.Set(float64(cachedCounts.MasterUserRecordCount))
	})
}

// DecrementMasterUserRecordCount decreases the number of MasterUserRecord in the cached counter
func DecrementMasterUserRecordCount(log logr.Logger) {
	write(func() {
		if cachedCounts.MasterUserRecordCount != 0 || !cachedCounts.initialized {
			cachedCounts.MasterUserRecordCount--
		} else {
			log.Error(fmt.Errorf("the count of MasterUserRecords is zero"),
				"unable to decrement the number of MasterUserRecords")
		}
		metrics.MasterUserRecordGauge.Set(float64(cachedCounts.MasterUserRecordCount))
	})
}

// IncrementUserAccountCount increments the number of UserAccount for the given member cluster in the cached counter
func IncrementUserAccountCount(clusterName string) {
	write(func() {
		cachedCounts.UserAccountsPerClusterCounts[clusterName]++
	})
}

// DecrementUserAccountCount decreases the number of UserAccount for the given member cluster in the cached counter
func DecrementUserAccountCount(log logr.Logger, clusterName string) {
	write(func() {
		if cachedCounts.UserAccountsPerClusterCounts[clusterName] != 0 || !cachedCounts.initialized {
			cachedCounts.UserAccountsPerClusterCounts[clusterName]--
		} else {
			log.Error(fmt.Errorf("the count of UserAccounts is zero"),
				"unable to decrement the number of UserAccounts for the given cluster", "cluster", clusterName)
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
func Synchronize(cl client.Client, toolchainStatus *v1alpha1.ToolchainStatus) error {
	cachedCounts.Lock()
	defer cachedCounts.Unlock()
	if shouldLoadCurrentMURsAndUserAccounts(toolchainStatus) {
		if err := loadCurrentMURsAndUserAccounts(cl, toolchainStatus.Namespace); err != nil {
			return err
		}
	}
	if toolchainStatus.Status.HostOperator == nil {
		toolchainStatus.Status.HostOperator = &v1alpha1.HostOperatorStatus{}
	}
	if !cachedCounts.initialized {
		cachedCounts.MasterUserRecordCount += toolchainStatus.Status.HostOperator.MasterUserRecordCount
		for _, memberStatus := range toolchainStatus.Status.Members {
			cachedCounts.UserAccountsPerClusterCounts[memberStatus.ClusterName] += memberStatus.UserAccountCount
		}
		cachedCounts.initialized = true
	}

	toolchainStatus.Status.HostOperator.MasterUserRecordCount = cachedCounts.MasterUserRecordCount
CachedCountsPerCluster:
	for clusterName, count := range cachedCounts.UserAccountsPerClusterCounts {
		for index, member := range toolchainStatus.Status.Members {
			if clusterName == member.ClusterName {
				toolchainStatus.Status.Members[index].UserAccountCount = count
				continue CachedCountsPerCluster
			}
		}
		toolchainStatus.Status.Members = append(toolchainStatus.Status.Members, v1alpha1.Member{
			ClusterName:      clusterName,
			UserAccountCount: count,
		})
	}
	metrics.MasterUserRecordGauge.Set(float64(cachedCounts.MasterUserRecordCount))
	return nil
}

func shouldLoadCurrentMURsAndUserAccounts(toolchainStatus *v1alpha1.ToolchainStatus) bool {
	return (toolchainStatus.Status.HostOperator == nil ||
		toolchainStatus.Status.HostOperator.MasterUserRecordCount == 0) &&
		!cachedCounts.initialized
}

func loadCurrentMURsAndUserAccounts(cl client.Client, namespace string) error {
	murs := &v1alpha1.MasterUserRecordList{}
	if err := cl.List(context.TODO(), murs, client.InNamespace(namespace)); err != nil {
		return err
	}
	reset()
	for _, mur := range murs.Items {
		cachedCounts.MasterUserRecordCount++
		for _, ua := range mur.Spec.UserAccounts {
			cachedCounts.UserAccountsPerClusterCounts[ua.TargetCluster]++
		}
	}
	cachedCounts.initialized = true
	return nil
}
