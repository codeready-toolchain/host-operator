package test

import (
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RegServiceAssertion struct {
	RegistrationService *toolchainv1alpha1.RegistrationService
	client              client.Client
	namespacedName      types.NamespacedName
	t                   *testing.T
}

func (a *RegServiceAssertion) loadRegServiceAssertion() error {
	if a.RegistrationService != nil {
		return nil
	}
	regServ := &toolchainv1alpha1.RegistrationService{}
	err := a.client.Get(context.TODO(), a.namespacedName, regServ)
	a.RegistrationService = regServ
	return err
}

func AssertThatRegistrationService(t *testing.T, name string, client client.Client) *RegServiceAssertion {
	return &RegServiceAssertion{
		client:         client,
		namespacedName: test.NamespacedName(test.HostOperatorNs, name),
		t:              t,
	}
}

func (a *RegServiceAssertion) HasConditions(expected ...toolchainv1alpha1.Condition) *RegServiceAssertion {
	err := a.loadRegServiceAssertion()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.RegistrationService.Status.Conditions, expected...)
	return a
}

func (a *RegServiceAssertion) HasImage(image string) *RegServiceAssertion {
	err := a.loadRegServiceAssertion()
	require.NoError(a.t, err)
	assert.Equal(a.t, image, a.RegistrationService.Spec.EnvironmentVariables["IMAGE"])
	return a
}

func (a *RegServiceAssertion) HasReplicas(replicas string) *RegServiceAssertion {
	err := a.loadRegServiceAssertion()
	require.NoError(a.t, err)
	assert.Equal(a.t, replicas, a.RegistrationService.Spec.EnvironmentVariables["REPLICAS"])
	return a
}

func (a *RegServiceAssertion) HasEnvironment(env string) *RegServiceAssertion {
	err := a.loadRegServiceAssertion()
	require.NoError(a.t, err)
	assert.Equal(a.t, env, a.RegistrationService.Spec.EnvironmentVariables["ENVIRONMENT"])
	return a
}

func (a *RegServiceAssertion) HasAuthLibraryUrl(libUrl string) *RegServiceAssertion {
	err := a.loadRegServiceAssertion()
	require.NoError(a.t, err)
	assert.Equal(a.t, libUrl, a.RegistrationService.Spec.EnvironmentVariables["AUTH_CLIENT_LIBRARY_URL"])
	return a
}

func (a *RegServiceAssertion) HasAuthPublicKeysUrl(publicKeysUrl string) *RegServiceAssertion {
	err := a.loadRegServiceAssertion()
	require.NoError(a.t, err)
	assert.Equal(a.t, publicKeysUrl, a.RegistrationService.Spec.EnvironmentVariables["AUTH_CLIENT_PUBLIC_KEYS_URL"])
	return a
}

func (a *RegServiceAssertion) HasAuthConfig(config string) *RegServiceAssertion {
	err := a.loadRegServiceAssertion()
	require.NoError(a.t, err)
	assert.Equal(a.t, config, a.RegistrationService.Spec.EnvironmentVariables["AUTH_CLIENT_CONFIG_RAW"])
	return a
}
