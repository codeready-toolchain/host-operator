package test

import (
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

type ExpectedNumberOfUserAccounts func() (string, int)

func UserAccountsForCluster(clusterName string, number int) ExpectedNumberOfUserAccounts {
	return func() (string, int) {
		return clusterName, number
	}
}

func AssertThatUninitializedCounterHas(t *testing.T, numberOfMurs int, numberOfUasPerCluster ...ExpectedNumberOfUserAccounts) {
	counts, err := counter.GetCounts()
	assert.EqualErrorf(t, err, "counter is not initialized", "should be error because counter hasn't been initialized yet")
	verifyCounts(t, counts, numberOfMurs, numberOfUasPerCluster...)
}

func AssertThatCounterHas(t *testing.T, numberOfMurs int, numberOfUasPerCluster ...ExpectedNumberOfUserAccounts) {
	counts, err := counter.GetCounts()
	require.NoError(t, err)
	verifyCounts(t, counts, numberOfMurs, numberOfUasPerCluster...)
}

func verifyCounts(t *testing.T, counts counter.Counts, numberOfMurs int, numberOfUasPerCluster ...ExpectedNumberOfUserAccounts) {
	assert.Equal(t, numberOfMurs, counts.MasterUserRecordCount)
	AssertMetricsGaugeEquals(t, numberOfMurs, metrics.MasterUserRecordGauge)
	assert.Len(t, counts.UserAccountsPerClusterCounts, len(numberOfUasPerCluster))
	for _, userAccountsForCluster := range numberOfUasPerCluster {
		clusterName, count := userAccountsForCluster()
		assert.Equal(t, count, counts.UserAccountsPerClusterCounts[clusterName])
		AssertMetricsGaugeEquals(t, count, metrics.UserAccountGaugeVec.WithLabelValues(clusterName))
	}
}

func CreateMultipleMurs(t *testing.T, prefix string, number int, targetCluster string) []runtime.Object {
	murs := make([]runtime.Object, number)
	for index := range murs {
		murs[index] = masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("%s%d", prefix, index), masteruserrecord.TargetCluster(targetCluster))
	}
	return murs
}

func InitializeCounter(t *testing.T, numberOfMurs int, numberOfUasPerCluster ...ExpectedNumberOfUserAccounts) *v1alpha1.ToolchainStatus {
	counter.Reset()
	return InitializeCounterWithoutReset(t, numberOfMurs, numberOfUasPerCluster...)
}
func InitializeCounterWithClient(t *testing.T, cl *test.FakeClient, numberOfMurs int, numberOfUasPerCluster ...ExpectedNumberOfUserAccounts) *v1alpha1.ToolchainStatus {
	counter.Reset()
	return initializeCounter(t, cl, numberOfMurs, numberOfUasPerCluster...)
}

func InitializeCounterWithoutReset(t *testing.T, numberOfMurs int, numberOfUasPerCluster ...ExpectedNumberOfUserAccounts) *v1alpha1.ToolchainStatus {
	return initializeCounter(t, test.NewFakeClient(t), numberOfMurs, numberOfUasPerCluster...)
}

func initializeCounter(t *testing.T, cl *test.FakeClient, numberOfMurs int, numberOfUasPerCluster ...ExpectedNumberOfUserAccounts) *v1alpha1.ToolchainStatus {
	metrics.MasterUserRecordGauge.Set(float64(numberOfMurs))
	if len(numberOfUasPerCluster) > 0 && numberOfMurs == 0 {
		require.FailNow(t, "When specifying number of UserAccounts per member cluster, you need to specify a count of MURs that is higher than zero")
	}
	toolchainStatus := &v1alpha1.ToolchainStatus{
		Status: v1alpha1.ToolchainStatusStatus{
			HostOperator: &v1alpha1.HostOperatorStatus{
				MasterUserRecordCount: numberOfMurs,
			},
		},
	}

	for _, uaForCluster := range numberOfUasPerCluster {
		clusterName, uaCount := uaForCluster()
		toolchainStatus.Status.Members = append(toolchainStatus.Status.Members, v1alpha1.Member{
			ClusterName:      clusterName,
			UserAccountCount: uaCount,
		})
	}

	err := counter.Synchronize(cl, toolchainStatus)
	require.NoError(t, err)
	t.Logf("MasterUserRecordGauge=%.0f", promtestutil.ToFloat64(metrics.MasterUserRecordGauge))
	return toolchainStatus
}
