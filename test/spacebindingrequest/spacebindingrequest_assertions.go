package spacebindingrequest

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Assertion struct {
	spaceBindingRequest *toolchainv1alpha1.SpaceBindingRequest
	client              runtimeclient.Client
	namespacedName      types.NamespacedName
	t                   test.T
}

func (a *Assertion) loadResource() error {
	spaceBindingRequest := &toolchainv1alpha1.SpaceBindingRequest{}
	err := a.client.Get(context.TODO(), a.namespacedName, spaceBindingRequest)
	a.spaceBindingRequest = spaceBindingRequest
	return err
}

// AssertThatSpaceBindingRequest helper func to begin with the assertions on a SpaceBindingRequests
func AssertThatSpaceBindingRequest(t test.T, namespace, name string, client runtimeclient.Client) *Assertion {
	return &Assertion{
		client:         client,
		namespacedName: test.NamespacedName(namespace, name),
		t:              t,
	}
}

func (a *Assertion) Exists() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	return a
}

func (a *Assertion) HasFinalizer() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Contains(a.t, a.spaceBindingRequest.Finalizers, toolchainv1alpha1.FinalizerName)
	return a
}

func (a *Assertion) HasNoFinalizers() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Empty(a.t, a.spaceBindingRequest.Finalizers)
	return a
}

func (a *Assertion) HasSpecMasterUserRecord(mur string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, mur, a.spaceBindingRequest.Spec.MasterUserRecord)
	return a
}

func (a *Assertion) HasSpecSpaceRole(role string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, role, a.spaceBindingRequest.Spec.SpaceRole)
	return a
}

func (a *Assertion) HasConditions(expected ...toolchainv1alpha1.Condition) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.spaceBindingRequest.Status.Conditions, expected...)
	return a
}

func (a *Assertion) HasLabelWithValue(key, value string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.NotNil(a.t, a.spaceBindingRequest.Labels)
	assert.Equal(a.t, value, a.spaceBindingRequest.Labels[key])
	return a
}

func (a *Assertion) DoesNotExist() *Assertion {
	err := a.loadResource()
	require.Error(a.t, err)
	require.True(a.t, errors.IsNotFound(err))
	return a
}
