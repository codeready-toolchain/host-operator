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

// AssertThatSpace helper func to begin with the assertions on a SpaceBinding
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
