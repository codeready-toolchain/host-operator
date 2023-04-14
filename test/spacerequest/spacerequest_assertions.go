package spacerequest

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Assertion struct {
	spaceRequest   *toolchainv1alpha1.SpaceRequest
	client         client.Client
	namespacedName types.NamespacedName
	t              test.T
}

func (a *Assertion) loadResource() error {
	spaceRequest := &toolchainv1alpha1.SpaceRequest{}
	err := a.client.Get(context.TODO(), a.namespacedName, spaceRequest)
	a.spaceRequest = spaceRequest
	return err
}

// AssertThatSpaceRequest helper func to begin with the assertions on a SpaceRequests
func AssertThatSpaceRequest(t test.T, namespace, name string, client client.Client) *Assertion {
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
	assert.Contains(a.t, a.spaceRequest.Finalizers, toolchainv1alpha1.FinalizerName)
	return a
}

func (a *Assertion) HasNoFinalizers() *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Empty(a.t, a.spaceRequest.Finalizers)
	return a
}

func (a *Assertion) HasSpecTargetClusterRoles(roles []string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, roles, a.spaceRequest.Spec.TargetClusterRoles)
	return a
}

func (a *Assertion) HasStatusTargetClusterURL(targetCluster string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, targetCluster, a.spaceRequest.Status.TargetClusterURL)
	return a
}

func (a *Assertion) HasNamespaceAccess(namespace, oldSecretRef string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.True(a.t, len(a.spaceRequest.Status.NamespaceAccess) > 0)
	assert.Equal(a.t, namespace, a.spaceRequest.Status.NamespaceAccess[0].Name)
	// check that the secret was created
	assert.NotEmpty(a.t, a.spaceRequest.Status.NamespaceAccess[0].SecretRef)
	secret := &corev1.Secret{}
	err = a.client.Get(context.TODO(), types.NamespacedName{
		Namespace: a.spaceRequest.Namespace,
		Name:      a.spaceRequest.Status.NamespaceAccess[0].SecretRef,
	}, secret)
	require.NoError(a.t, err)
	assert.NotEmpty(a.t, secret)
	if oldSecretRef != "" {
		// if old secret was provided we check that the name has changed,
		// meaning a new secret was created.
		assert.NotEqual(a.t, oldSecretRef, a.spaceRequest.Status.NamespaceAccess[0].SecretRef)
	}
	return a
}

func (a *Assertion) HasSpecTierName(tierName string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, tierName, a.spaceRequest.Spec.TierName)
	return a
}

func (a *Assertion) HasTargetClusterURL(targetCluster string) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, targetCluster, a.spaceRequest.Status.TargetClusterURL)
	return a
}

func (a *Assertion) HasConditions(expected ...toolchainv1alpha1.Condition) *Assertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.spaceRequest.Status.Conditions, expected...)
	return a
}

func (a *Assertion) DoesNotExist() *Assertion {
	err := a.loadResource()
	require.Error(a.t, err)
	require.True(a.t, errors.IsNotFound(err))
	return a
}
