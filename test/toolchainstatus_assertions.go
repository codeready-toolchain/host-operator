package test

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ToolchainStatusAssertion struct {
	toolchainStatus *toolchainv1alpha1.ToolchainStatus
	client          client.Client
	namespacedName  types.NamespacedName
	t               test.T
}

func (a *ToolchainStatusAssertion) loadToolchainStatus() error {
	if a.client != nil {
		toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
		err := a.client.Get(context.TODO(), a.namespacedName, toolchainStatus)
		a.toolchainStatus = toolchainStatus
		return err
	}
	return nil
}

func AssertThatToolchainStatus(t test.T, namespace, name string, client client.Client) *ToolchainStatusAssertion {
	return &ToolchainStatusAssertion{
		client:         client,
		namespacedName: test.NamespacedName(namespace, name),
		t:              t,
	}
}

func AssertThatGivenToolchainStatus(t test.T, toolchainStatus *toolchainv1alpha1.ToolchainStatus) *ToolchainStatusAssertion {
	return &ToolchainStatusAssertion{
		t:               t,
		toolchainStatus: toolchainStatus,
	}
}

func (a *ToolchainStatusAssertion) Exists() *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	return a
}

func (a *ToolchainStatusAssertion) HasConditions(expected ...toolchainv1alpha1.Condition) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.toolchainStatus.Status.Conditions, expected...)
	return a
}

func (a *ToolchainStatusAssertion) HasHostOperatorStatus(expected toolchainv1alpha1.HostOperatorStatus) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	require.NotNil(a.t, *a.toolchainStatus.Status.HostOperator)
	test.AssertHostOperatorStatusMatch(a.t, *a.toolchainStatus.Status.HostOperator, expected)
	return a
}

func (a *ToolchainStatusAssertion) HasUsersPerActivationsAndDomain(expectedMetric toolchainv1alpha1.Metric) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	require.NotEmpty(a.t, a.toolchainStatus.Status.Metrics[toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey])
	assert.Equal(a.t, expectedMetric, a.toolchainStatus.Status.Metrics[toolchainv1alpha1.UserSignupsPerActivationAndDomainMetricKey])
	return a
}

func (a *ToolchainStatusAssertion) HasMasterUserRecordsPerDomain(expectedMetric toolchainv1alpha1.Metric) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	require.NotEmpty(a.t, a.toolchainStatus.Status.Metrics[toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey])
	assert.Equal(a.t, expectedMetric, a.toolchainStatus.Status.Metrics[toolchainv1alpha1.MasterUserRecordsPerDomainMetricKey])
	return a
}

func (a *ToolchainStatusAssertion) HasNoMetric(key string) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	require.Empty(a.t, a.toolchainStatus.Status.Metrics[key])
	return a
}

func (a *ToolchainStatusAssertion) HasUserAccountCount(memberClusterName string, expectedCount int) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	require.NotNil(a.t, *a.toolchainStatus.Status.HostOperator)
	for _, member := range a.toolchainStatus.Status.Members {
		if member.ClusterName == memberClusterName {
			assert.Equal(a.t, expectedCount, member.UserAccountCount)
			return a
		}
	}
	require.Fail(a.t, fmt.Sprintf("cluster with the name %s wasn't found", memberClusterName))
	return a
}

func (a *ToolchainStatusAssertion) HasMemberClusterStatus(expected ...toolchainv1alpha1.Member) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	test.AssertMembersMatch(a.t, a.toolchainStatus.Status.Members, expected...)

	return a
}

func (a *ToolchainStatusAssertion) HasRegistrationServiceStatus(expected toolchainv1alpha1.HostRegistrationServiceStatus) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	test.AssertRegistrationServiceStatusMatch(a.t, *a.toolchainStatus.Status.RegistrationService, expected)
	return a
}

func (a *ToolchainStatusAssertion) ReadyConditionLastTransitionTimeEqual(expected metav1.Time) *ToolchainStatusAssertion {
	return a.readyConditionLastTransitionTimeEqual(expected, true)
}

func (a *ToolchainStatusAssertion) ReadyConditionLastTransitionTimeNotEqual(expected metav1.Time) *ToolchainStatusAssertion {
	return a.readyConditionLastTransitionTimeEqual(expected, false)
}

func (a *ToolchainStatusAssertion) readyConditionLastTransitionTimeEqual(expected metav1.Time, equal bool) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)

	ready, found := condition.FindConditionByType(a.toolchainStatus.Status.Conditions, toolchainv1alpha1.ConditionReady)
	require.True(a.t, found)
	if equal {
		assert.Equal(a.t, expected.Unix(), ready.LastTransitionTime.Unix())
	} else {
		assert.NotEqual(a.t, expected.Unix(), ready.LastTransitionTime.Unix())
	}
	return a
}

func (a *ToolchainStatusAssertion) ReadyConditionLastUpdatedTimeEqual(expected metav1.Time) *ToolchainStatusAssertion {
	return a.readyConditionLastUpdatedTimeEqual(expected, true)
}

func (a *ToolchainStatusAssertion) ReadyConditionLastUpdatedTimeNotEqual(expected metav1.Time) *ToolchainStatusAssertion {
	return a.readyConditionLastUpdatedTimeEqual(expected, false)
}

func (a *ToolchainStatusAssertion) readyConditionLastUpdatedTimeEqual(expected metav1.Time, equal bool) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)

	ready, found := condition.FindConditionByType(a.toolchainStatus.Status.Conditions, toolchainv1alpha1.ConditionReady)
	require.True(a.t, found)
	if equal {
		assert.Equal(a.t, expected.Unix(), ready.LastUpdatedTime.Unix())
	} else {
		assert.NotEqual(a.t, expected.Unix(), ready.LastUpdatedTime.Unix())
	}
	return a
}
func (a *ToolchainStatusAssertion) ReadyConditionLastUpdatedTimeNotEmpty() *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)

	ready, found := condition.FindConditionByType(a.toolchainStatus.Status.Conditions, toolchainv1alpha1.ConditionReady)
	require.True(a.t, found)
	assert.NotEmpty(a.t, ready.LastUpdatedTime)
	return a
}

func (a *ToolchainStatusAssertion) ReadyConditionLastTransitionTimeNotEmpty() *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)

	ready, found := condition.FindConditionByType(a.toolchainStatus.Status.Conditions, toolchainv1alpha1.ConditionReady)
	require.True(a.t, found)
	assert.NotEmpty(a.t, ready.LastTransitionTime)
	return a
}
