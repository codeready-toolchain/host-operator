package test

import (
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

type CounterAssertion struct {
	t      test.T
	counts counter.Counts
}

func AssertThatCounters(t *testing.T) *CounterAssertion {
	counts, err := counter.GetCounts()
	require.NoError(t, err)
	return &CounterAssertion{
		t:      t,
		counts: counts,
	}
}

func AssertThatUnitializedCounters(t *testing.T) *CounterAssertion {
	counts, err := counter.GetCounts()
	require.EqualErrorf(t, err, "counter is not initialized", "should be error because counter hasn't been initialized yet")
	return &CounterAssertion{
		t:      t,
		counts: counts,
	}
}

func (a *CounterAssertion) HaveMasterUserRecords(number int) *CounterAssertion {
	assert.Equal(a.t, number, a.counts.MasterUserRecordCount)
	return a
}

func (a *CounterAssertion) HaveUserAccountsForCluster(clusterName string, number int) *CounterAssertion {
	assert.Equal(a.t, number, a.counts.UserAccountsPerClusterCounts[clusterName])
	return a
}

func CreateMultipleMurs(t *testing.T, prefix string, number int, targetCluster string) []runtime.Object {
	murs := make([]runtime.Object, number)
	for index := range murs {
		murs[index] = masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("%s%d", prefix, index), masteruserrecord.TargetCluster(targetCluster))
	}
	return murs
}

func InitializeCounters(t *testing.T, toolchainStatus *v1alpha1.ToolchainStatus, initObjs ...runtime.Object) {
	counter.Reset()
	t.Cleanup(counter.Reset)
	initializeCounters(t, commontest.NewFakeClient(t, initObjs...), toolchainStatus)
}

func InitializeCountersWithoutReset(t *testing.T, toolchainStatus *v1alpha1.ToolchainStatus) {
	t.Cleanup(counter.Reset)
	initializeCounters(t, commontest.NewFakeClient(t), toolchainStatus)
}

func initializeCounters(t *testing.T, cl *commontest.FakeClient, toolchainStatus *v1alpha1.ToolchainStatus) {
	if toolchainStatus.Status.HostOperator != nil {
		metrics.MasterUserRecordGauge.Set(float64(toolchainStatus.Status.HostOperator.MasterUserRecordCount))
	}
	t.Logf("toolchainStatus members: %v", toolchainStatus.Status.Members)
	err := counter.Synchronize(cl, toolchainStatus)
	require.NoError(t, err)
	t.Logf("MasterUserRecordGauge=%.0f", promtestutil.ToFloat64(metrics.MasterUserRecordGauge))
}
