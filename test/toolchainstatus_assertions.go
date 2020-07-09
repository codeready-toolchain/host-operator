package test

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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

func (a *ToolchainStatusAssertion) HasHostOperatorConditionErrorMsg(msg string) *ToolchainStatusAssertion {
	err := a.loadToolchainStatus()
	require.NoError(a.t, err)
	require.Contains(a.t, a.toolchainStatus.Status.HostOperator.Conditions[0].Message, msg)
	return a
}

func ComponentsReady() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.ToolchainStatusReasonAllComponentsReady,
	}
}

func ComponentsNotReady(components ...string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.ToolchainStatusReasonComponentsNotReady,
		Message: fmt.Sprintf("components not ready: %v", components),
	}
}
