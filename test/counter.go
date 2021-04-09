package test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	usersignup "github.com/codeready-toolchain/host-operator/test/usersignups"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

type BaseValue func(*v1alpha1.ToolchainStatus)

func MasterUserRecords(number int) BaseValue {
	return func(status *v1alpha1.ToolchainStatus) {
		status.Status.HostOperator = &v1alpha1.HostOperatorStatus{
			MasterUserRecordCount: number,
		}
	}
}

func UserAccountsForCluster(clusterName string, number int) BaseValue {
	return func(status *v1alpha1.ToolchainStatus) {
		status.Status.Members = append(status.Status.Members, v1alpha1.Member{
			ClusterName:      clusterName,
			UserAccountCount: number,
		})
	}
}

func UsersPerActivations(values v1alpha1.Metric) BaseValue {
	return func(status *v1alpha1.ToolchainStatus) {
		if status.Status.Metrics == nil {
			status.Status.Metrics = map[string]v1alpha1.Metric{}
		}
		status.Status.Metrics[v1alpha1.UsersPerActivationMetricKey] = values
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
	// MasterUserRecordCount
	require.NotNil(t, toolchainStatus.Status.HostOperator)
	assert.Equal(t, toolchainStatus.Status.HostOperator.MasterUserRecordCount, counts.MasterUserRecordCount)
	AssertMetricsGaugeEquals(t, toolchainStatus.Status.HostOperator.MasterUserRecordCount, metrics.MasterUserRecordGauge)

	// UserAccountsPerClusterCounts
	assert.Len(t, counts.UserAccountsPerClusterCounts, len(toolchainStatus.Status.Members))
	for _, member := range toolchainStatus.Status.Members {
		assert.Equal(t, member.UserAccountCount, counts.UserAccountsPerClusterCounts[member.ClusterName])
		AssertMetricsGaugeEquals(t, member.UserAccountCount, metrics.UserAccountGaugeVec.WithLabelValues(member.ClusterName))
	}

	// UsersPerActivationCounts (if applicable)
	if usersPerActivations, ok := toolchainStatus.Status.Metrics[v1alpha1.UsersPerActivationMetricKey]; ok {
		assert.Equal(t, v1alpha1.Metric(counts.UsersPerActivationCounts), usersPerActivations)
		for activations, users := range usersPerActivations {
			AssertMetricsGaugeEquals(t, users, metrics.UsersPerActivationGaugeVec.WithLabelValues(activations))
		}
	}

}

func CreateMultipleUserSignups(prefix string, number int) []runtime.Object {
	usersignups := make([]runtime.Object, number)
	for index := range usersignups {
		usersignups[index] = usersignup.NewUserSignup(
			fmt.Sprintf("%s%d", prefix, index),
			usersignup.WithAnnotation(v1alpha1.UserSignupActivationCounterAnnotationKey, strconv.Itoa(index+1)),
		)
	}
	return usersignups
}

func CreateMultipleMurs(t *testing.T, prefix string, number int, targetCluster string) []runtime.Object {
	murs := make([]runtime.Object, number)
	for index := range murs {
		murs[index] = masteruserrecord.NewMasterUserRecord(t, fmt.Sprintf("%s%d", prefix, index), masteruserrecord.TargetCluster(targetCluster))
	}
	return murs
}

func InitializeCounter(t *testing.T, baseValues ...BaseValue) *v1alpha1.ToolchainStatus {
	counter.Reset()
	return InitializeCounterWithoutReset(t, baseValues...)
}

func InitializeCounterWithBaseValues(t *testing.T, cl *test.FakeClient, baseValues ...BaseValue) *v1alpha1.ToolchainStatus {
	counter.Reset()
	return initializeCounter(t, cl, baseValues...)
}

func InitializeCounterWithoutReset(t *testing.T, baseValues ...BaseValue) *v1alpha1.ToolchainStatus {
	return initializeCounter(t, test.NewFakeClient(t), baseValues...)
}

func initializeCounter(t *testing.T, cl *test.FakeClient, baseValues ...BaseValue) *v1alpha1.ToolchainStatus {
	toolchainStatus := &v1alpha1.ToolchainStatus{
		Status: v1alpha1.ToolchainStatusStatus{},
	}
	for _, apply := range baseValues {
		apply(toolchainStatus)
	}
	// TODO: is it needed?
	if toolchainStatus.Status.HostOperator != nil {
		metrics.MasterUserRecordGauge.Set(float64(toolchainStatus.Status.HostOperator.MasterUserRecordCount))
	}
	t.Logf("toolchainStatus members: %v", toolchainStatus.Status.Members)
	err := counter.Synchronize(cl, toolchainStatus)
	require.NoError(t, err)
	t.Logf("MasterUserRecordGauge=%.0f", promtestutil.ToFloat64(metrics.MasterUserRecordGauge))
	return toolchainStatus
}
