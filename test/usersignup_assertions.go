package test

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/usersignup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type UserSignupAssertion struct {
	usersignup     *toolchainv1alpha1.UserSignup
	client         runtimeclient.Client
	namespacedName types.NamespacedName
	t              test.T
}

func (a *UserSignupAssertion) loadUserSignup() error {
	if a.client != nil {
		usersignup := &toolchainv1alpha1.UserSignup{}
		err := a.client.Get(context.TODO(), a.namespacedName, usersignup)
		a.usersignup = usersignup
		return err
	}
	return nil
}

func AssertThatUserSignup(t test.T, namespace, name string, client runtimeclient.Client) *UserSignupAssertion {
	return &UserSignupAssertion{
		client:         client,
		namespacedName: test.NamespacedName(namespace, name),
		t:              t,
	}
}

func (a *UserSignupAssertion) Get() *toolchainv1alpha1.UserSignup {
	err := a.loadUserSignup()
	require.NoError(a.t, err)
	return a.usersignup
}

// Checks that the CompliantUsername matches the given string and is less than the maxLegth (20 characsters)
func (a *UserSignupAssertion) HasCompliantUsername(name string) *UserSignupAssertion {
	err := a.loadUserSignup()
	require.NoError(a.t, err)
	assert.Equal(a.t, name, a.usersignup.Status.CompliantUsername)
	assert.LessOrEqual(a.t, len(a.usersignup.Status.CompliantUsername), usersignup.MaxLength)
	return a
}

// Checks that the HomeSpace matches the given string
func (a *UserSignupAssertion) HasHomeSpace(name string) *UserSignupAssertion {
	err := a.loadUserSignup()
	require.NoError(a.t, err)
	assert.Equal(a.t, name, a.usersignup.Status.HomeSpace)
	return a
}

func (a *UserSignupAssertion) HasLabel(key, value string) *UserSignupAssertion {
	err := a.loadUserSignup()
	require.NoError(a.t, err)
	v, found := a.usersignup.Labels[key]
	require.True(a.t, found)
	assert.Equal(a.t, value, v)
	return a
}

func (a *UserSignupAssertion) HasAnnotation(key, value string) *UserSignupAssertion {
	err := a.loadUserSignup()
	require.NoError(a.t, err)
	v, found := a.usersignup.Annotations[key]
	require.True(a.t, found)
	assert.Equal(a.t, value, v)
	return a
}

func (a *UserSignupAssertion) HasNoAnnotation(key string) *UserSignupAssertion {
	err := a.loadUserSignup()
	require.NoError(a.t, err)
	_, found := a.usersignup.Annotations[key]
	assert.False(a.t, found)
	return a
}
