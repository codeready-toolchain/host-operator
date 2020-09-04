package test

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

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
	toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
	err := a.client.Get(context.TODO(), a.namespacedName, toolchainStatus)
	a.toolchainStatus = toolchainStatus
	return err
}

func AssertThatToolchainStatus(t test.T, namespace, name string, client client.Client) *ToolchainStatusAssertion {
	return &ToolchainStatusAssertion{
		client:         client,
		namespacedName: test.NamespacedName(namespace, name),
		t:              t,
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
