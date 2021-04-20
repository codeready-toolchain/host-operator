package test

import (
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

type BaseValue func(*toolchainv1alpha1.ToolchainStatus)

func MasterUserRecords(number int) BaseValue {
	return func(status *toolchainv1alpha1.ToolchainStatus) {
		status.Status.HostOperator = &toolchainv1alpha1.HostOperatorStatus{
			MasterUserRecordCount: number,
		}
	}
}

func UserAccountsForCluster(clusterName string, number int) BaseValue {
	return func(status *toolchainv1alpha1.ToolchainStatus) {
		status.Status.Members = append(status.Status.Members, v1alpha1.Member{
			ClusterName:      clusterName,
			UserAccountCount: number,
		})
	}
}

func AssertThatUninitializedCounterHas(t *testing.T, baseValues ...BaseValue) {
	counts, err := counter.GetCounts()
	assert.EqualErrorf(t, err, "counter is not initialized", "should be error because counter hasn't been initialized yet")
	verifyCountsAndMetrics(t, counts, baseValues...)
}

func AssertThatCounterHas(t *testing.T, baseValues ...BaseValue) {
	counts, err := counter.GetCounts()
	require.NoError(t, err)
	verifyCountsAndMetrics(t, counts, baseValues...)
}

func verifyCountsAndMetrics(t *testing.T, counts counter.Counts, baseValues ...BaseValue) {

	toolchainStatus := &v1alpha1.ToolchainStatus{
		Status: v1alpha1.ToolchainStatusStatus{},
	}
	for _, apply := range baseValues {
		apply(toolchainStatus)
	}
	require.NotNil(t, toolchainStatus.Status.HostOperator)
	assert.Equal(t, toolchainStatus.Status.HostOperator.MasterUserRecordCount, counts.MasterUserRecordCount)
	AssertMetricsGaugeEquals(t, toolchainStatus.Status.HostOperator.MasterUserRecordCount, metrics.MasterUserRecordGauge)

	assert.Len(t, counts.UserAccountsPerClusterCounts, len(toolchainStatus.Status.Members))
	for _, member := range toolchainStatus.Status.Members {
		assert.Equal(t, member.UserAccountCount, counts.UserAccountsPerClusterCounts[member.ClusterName])
		AssertMetricsGaugeEquals(t, member.UserAccountCount, metrics.UserAccountGaugeVec.WithLabelValues(member.ClusterName))
	}
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
