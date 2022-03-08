package space

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Assertion struct {
	space          *toolchainv1alpha1.Space
	client         client.Client
	namespacedName types.NamespacedName
	t              test.T
}

func (a *Assertion) loadResource() error {
	space := &toolchainv1alpha1.Space{}
	err := a.client.Get(context.TODO(), a.namespacedName, space)
	a.space = space
	return err
}

// AssertThatSpace helper func to begin with the assertions on a Space
func AssertThatSpace(t test.T, namespace, name string, client client.Client) *Assertion {
	return &Assertion{
		client:         client,
		namespacedName: test.NamespacedName(namespace, name),
		t:              t,
	}
}

func (a *Assertion) Get() *toolchainv1alpha1.Space {
	err := a.loadResource()
	require.NoError(a.t, err)
	return a.space
}

func (a *Assertion) Exists() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	return a
}

func (a *Assertion) DoesNotExist() *Assertion {
	err := a.loadResource()
	require.Error(a.t, err)
	require.True(a.t, errors.IsNotFound(err))
	return a
}

func (a *Assertion) HasFinalizer() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Contains(a.t, a.space.Finalizers, toolchainv1alpha1.FinalizerName)
	return a
}

func (a *Assertion) HasNoFinalizers() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Empty(a.t, a.space.Finalizers)
	return a
}

func (a *Assertion) HasTier(tierName string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, tierName, a.space.Spec.TierName)
	return a
}

func (a *Assertion) HasLabelWithValue(key, value string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.NotNil(a.t, a.space.Labels)
	assert.Equal(a.t, value, a.space.Labels[key])
	return a
}

func (a *Assertion) HasMatchingTierLabelForTier(tier *toolchainv1alpha1.NSTemplateTier) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	key := tierutil.TemplateTierHashLabelKey(tier.Name)
	require.Contains(a.t, a.space.Labels, key)
	expectedHash, err := tierutil.ComputeHashForNSTemplateTier(tier)
	require.NoError(a.t, err)
	assert.Equal(a.t, expectedHash, a.space.Labels[key])
	return a
}

func (a *Assertion) HasStateLabel(stateValue string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.NotNil(a.t, a.space.Labels)
	assert.Equal(a.t, stateValue, a.space.Labels[toolchainv1alpha1.SpaceStateLabelKey])
	return a
}

func (a *Assertion) DoesNotHaveLabel(key string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.NotContains(a.t, a.space.Labels, key)
	return a
}

func (a *Assertion) HasNoSpecTargetCluster() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Empty(a.t, a.space.Spec.TargetCluster)
	return a
}

func (a *Assertion) HasSpecTargetCluster(targetCluster string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, targetCluster, a.space.Spec.TargetCluster)
	return a
}

func (a *Assertion) HasNoStatusTargetCluster() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Empty(a.t, a.space.Status.TargetCluster)
	return a
}

func (a *Assertion) HasStatusTargetCluster(targetCluster string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, targetCluster, a.space.Status.TargetCluster)
	return a
}

func (a *Assertion) HasConditions(expected ...toolchainv1alpha1.Condition) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.space.Status.Conditions, expected...)
	return a
}

func (a *Assertion) HasNoConditions() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Empty(a.t, a.space.Status.Conditions)
	return a
}

func Provisioning() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.SpaceProvisioningReason,
	}
}

func ProvisioningPending(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SpaceProvisioningPendingReason,
		Message: msg,
	}
}

func ProvisioningFailed(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
		Message: msg,
	}
}

func Retargeting() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.SpaceRetargetingReason,
	}
}

func RetargetingFailed(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SpaceRetargetingFailedReason,
		Message: msg,
	}
}

func Updating() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.SpaceUpdatingReason,
	}
}

func UnableToCreateNSTemplateSet(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SpaceUnableToCreateNSTemplateSetReason,
		Message: msg,
	}
}

func UnableToUpdateNSTemplateSet(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SpaceUnableToUpdateNSTemplateSetReason,
		Message: msg,
	}
}

func Ready() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.SpaceProvisionedReason,
	}
}

func Terminating() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.SpaceTerminatingReason,
	}
}

func TerminatingFailed(msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SpaceTerminatingFailedReason,
		Message: msg,
	}
}

// Assertions on multiple Spaces at once
type SpacesAssertion struct {
	spaces    *toolchainv1alpha1.SpaceList
	client    client.Client
	namespace string
	t         test.T
}

func AssertThatSpaces(t test.T, client client.Client) *SpacesAssertion {
	return &SpacesAssertion{
		client:    client,
		namespace: test.HostOperatorNs,
		t:         t,
	}
}

func (a *SpacesAssertion) loadSpaces() error {
	spaces := &toolchainv1alpha1.SpaceList{}
	err := a.client.List(context.TODO(), spaces, client.InNamespace(a.namespace))
	a.spaces = spaces
	return err
}

func (a *SpacesAssertion) HaveCount(count int) *SpacesAssertion {
	err := a.loadSpaces()
	require.NoError(a.t, err)
	require.Len(a.t, a.spaces.Items, count)
	return a
}
