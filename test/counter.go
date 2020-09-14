package test

import (
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
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
	assert.NoError(t, err)
	verifyCounts(t, counts, numberOfMurs, numberOfUasPerCluster...)
}

func verifyCounts(t *testing.T, counts counter.Counts, numberOfMurs int, numberOfUasPerCluster ...ExpectedNumberOfUserAccounts) {
	assert.Equal(t, numberOfMurs, counts.MasterUserRecordCount)
	assert.Len(t, counts.UserAccountsPerClusterCounts, len(numberOfUasPerCluster))
	for _, userAccountsForCluster := range numberOfUasPerCluster {
		clusterName, count := userAccountsForCluster()
		assert.Equal(t, count, counts.UserAccountsPerClusterCounts[clusterName])
	}
}

func CreateMultipleMurs(t *testing.T, number int) []runtime.Object {
	murs := make([]runtime.Object, number)
	for index := range murs {
		murs[index] = masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("johny-%d", index), masteruserrecord.TargetCluster("member-cluster"))
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
	return toolchainStatus
}
