package test

import (
	"context"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigMapAssertion struct {
	configmap      *corev1.ConfigMap
	client         runtimeclient.Client
	namespacedName types.NamespacedName
	t              test.T
}

func (a *ConfigMapAssertion) loadConfigMap() error {
	if a.client != nil {
		configmap := &corev1.ConfigMap{}
		err := a.client.Get(context.TODO(), a.namespacedName, configmap)
		a.configmap = configmap
		return err
	}
	return nil
}

func AssertThatConfigMap(t test.T, namespace, name string, client runtimeclient.Client) *ConfigMapAssertion {
	return &ConfigMapAssertion{
		client:         client,
		namespacedName: test.NamespacedName(namespace, name),
		t:              t,
	}
}

func (a *ConfigMapAssertion) Exists() *ConfigMapAssertion {
	err := a.loadConfigMap()
	require.NoError(a.t, err)
	return a
}

func (a *ConfigMapAssertion) HasOwner(owner runtime.Object) *ConfigMapAssertion {
	err := a.loadConfigMap()
	require.NoError(a.t, err)
	assertOwner(a.t, a.configmap.ObjectMeta, owner)
	return a
}

func (a *ConfigMapAssertion) HasData(data map[string]string) *ConfigMapAssertion {
	err := a.loadConfigMap()
	require.NoError(a.t, err)
	require.Len(a.t, a.configmap.Data, len(data))
	for k, v := range data {
		require.Contains(a.t, a.configmap.Data, k)
		assert.Equal(a.t, v, a.configmap.Data[k])
	}
	return a
}
