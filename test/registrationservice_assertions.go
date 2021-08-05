package test

import (
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
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
