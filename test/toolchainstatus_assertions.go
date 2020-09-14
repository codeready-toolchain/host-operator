package test

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
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

func (a *ToolchainStatusAssertion) HasCondition(expected toolchainv1alpha1.Condition) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.toolchainStatus.Status.Conditions, expected)
	return a
}

func (a *ToolchainStatusAssertion) HasHostOperatorStatus(expected toolchainv1alpha1.HostOperatorStatus) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	require.NotNil(a.t, *a.toolchainStatus.Status.HostOperator)
	test.AssertHostOperatorStatusMatch(a.t, *a.toolchainStatus.Status.HostOperator, expected)
	return a
}

func (a *ToolchainStatusAssertion) HasMurCount(expectedCount int) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	require.NotNil(a.t, *a.toolchainStatus.Status.HostOperator)
	assert.Equal(a.t, expectedCount, a.toolchainStatus.Status.HostOperator.MasterUserRecordCount)
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

func (a *ToolchainStatusAssertion) HasMemberStatus(expected ...toolchainv1alpha1.Member) *ToolchainStatusAssertion {
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
