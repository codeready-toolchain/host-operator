package test

import (
	"context"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceAccountAssertion struct {
	serviceaccount *corev1.ServiceAccount
	client         client.Client
	namespacedName types.NamespacedName
	t              test.T
}

func (a *ServiceAccountAssertion) loadServiceAccount() error {
	if a.client != nil {
		serviceaccount := &corev1.ServiceAccount{}
		err := a.client.Get(context.TODO(), a.namespacedName, serviceaccount)
		a.serviceaccount = serviceaccount
		return err
	}
	return nil
}

func AssertThatServiceAccount(t test.T, namespace, name string, client client.Client) *ServiceAccountAssertion {
	return &ServiceAccountAssertion{
		client:         client,
		namespacedName: test.NamespacedName(namespace, name),
		t:              t,
	}
}

func (a *ServiceAccountAssertion) Exists() *ServiceAccountAssertion {
	err := a.loadServiceAccount()
	require.NoError(a.t, err)
	return a
}

func (a *ServiceAccountAssertion) HasOwner(owner runtime.Object) *ServiceAccountAssertion {
	err := a.loadServiceAccount()
	require.NoError(a.t, err)
	assertOwner(a.t, a.serviceaccount.ObjectMeta, owner)
	return a
}

func assertOwner(t test.T, objectMeta metav1.ObjectMeta, owner runtime.Object) {
	require.Len(t, objectMeta.OwnerReferences, 1)
	assert.Equal(t, owner.GetObjectKind().GroupVersionKind().Kind, objectMeta.OwnerReferences[0].Kind)
	uOwner, err := runtime.DefaultUnstructuredConverter.ToUnstructured(owner)
	require.NoError(t, err)
	name, found, err := unstructured.NestedString(uOwner, "metadata", "name")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, name, objectMeta.Name)
}
