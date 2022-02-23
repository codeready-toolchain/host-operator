package spacebinding

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Assertion struct {
	spaceBinding   *toolchainv1alpha1.SpaceBinding
	client         client.Client
	namespacedName types.NamespacedName
	t              test.T
}

func (a *Assertion) loadResource() error {
	tier := &toolchainv1alpha1.SpaceBinding{}
	err := a.client.Get(context.TODO(), a.namespacedName, tier)
	a.spaceBinding = tier
	return err
}

// AssertThatSpaceBinding helper func to begin with the assertions on a SpaceBinding
func AssertThatSpaceBinding(t test.T, namespace, name string, client client.Client) *Assertion {
	return &Assertion{
		client:         client,
		namespacedName: test.NamespacedName(namespace, name),
		t:              t,
	}
}

func (a *Assertion) Get() *toolchainv1alpha1.SpaceBinding {
	err := a.loadResource()
	require.NoError(a.t, err)
	return a.spaceBinding
}

func (a *Assertion) Exists() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	return a
}

func (a *Assertion) DoesNotExist() *Assertion {
	err := a.loadResource()
	require.Error(a.t, err)
	assert.True(a.t, errors.IsNotFound(err))
	return a
}

func (a *Assertion) HasLabelWithValue(key, value string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.NotNil(a.t, a.spaceBinding.Labels)
	assert.Equal(a.t, value, a.spaceBinding.Labels[key])
	return a
}

func (a *Assertion) HasSpec(murName, spaceName, spaceRole string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, murName, a.spaceBinding.Spec.MasterUserRecord)
	assert.Equal(a.t, spaceName, a.spaceBinding.Spec.Space)
	assert.Equal(a.t, spaceRole, a.spaceBinding.Spec.SpaceRole)
	return a
}

// Assertions on multiple SpaceBindings at once
type SpaceBindingsAssertion struct {
	spacebindings *toolchainv1alpha1.SpaceBindingList
	client        client.Client
	namespace     string
	t             test.T
}

func AssertThatSpaceBindings(t test.T, client client.Client) *SpaceBindingsAssertion {
	return &SpaceBindingsAssertion{
		client:    client,
		namespace: test.HostOperatorNs,
		t:         t,
	}
}

func (a *SpaceBindingsAssertion) loadSpaceBindings() error {
	spacebindings := &toolchainv1alpha1.SpaceBindingList{}
	err := a.client.List(context.TODO(), spacebindings, client.InNamespace(a.namespace))
	a.spacebindings = spacebindings
	return err
}

func (a *SpaceBindingsAssertion) Get() *toolchainv1alpha1.SpaceBindingList {
	err := a.loadSpaceBindings()
	require.NoError(a.t, err)
	return a.spacebindings
}

func (a *SpaceBindingsAssertion) HaveCount(count int) *SpaceBindingsAssertion {
	err := a.loadSpaceBindings()
	require.NoError(a.t, err)
	require.Len(a.t, a.spacebindings.Items, count)
	return a
}
