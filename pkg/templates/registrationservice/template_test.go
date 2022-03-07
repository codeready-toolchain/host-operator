package registrationservice

import (
	"fmt"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/template"
	routev1 "github.com/openshift/api/route/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func TestDeploymentAssetContainsAllNecessaryInformation(t *testing.T) {
	// given
	s := runtime.NewScheme()
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	deploymentTemplate, err := GetDeploymentTemplate()
	require.NoError(t, err)
	vars := getVars("my-namespace", "quay.io/cr-t/registration-service:123", "10")
	processor := template.NewProcessor(s)

	// when
	toolchainObjects, err := processor.Process(deploymentTemplate, vars)

	// then
	require.NoError(t, err)
	deploymentFound := false
	saFound := false
	roleFound := false
	roleBindingFound := false
	apiRouteFound := false
	for _, toolchainObject := range toolchainObjects {
		assert.Equal(t, "my-namespace", toolchainObject.GetNamespace())
		gvk, err := apiutil.GVKForObject(toolchainObject, s)
		require.NoError(t, err)

		switch gvk {
		case appsv1.SchemeGroupVersion.WithKind("Deployment"):
			deploymentFound = true
			deployment := fmt.Sprintf("%+v", toolchainObject)
			assert.Contains(t, deployment, "replicas:10")
			assert.Contains(t, deployment, "image:quay.io/cr-t/registration-service:123")

		case corev1.SchemeGroupVersion.WithKind("ServiceAccount"):
			saFound = true
		case rbacv1.SchemeGroupVersion.WithKind("Role"):
			roleFound = true
		case rbacv1.SchemeGroupVersion.WithKind("RoleBinding"):
			roleBindingFound = true
		case routev1.GroupVersion.WithKind("Route"):
			if toolchainObject.GetName() == ProxyRouteName {
				apiRouteFound = true
			}
		}
	}
	assert.True(t, deploymentFound, "a Deployment wasn't found")
	assert.True(t, saFound, "a ServiceAccount wasn't found")
	assert.True(t, roleFound, "a Role wasn't found")
	assert.True(t, roleBindingFound, "a RoleBinding wasn't found")
	assert.True(t, apiRouteFound, "a proxy route with the name 'api' wasn't found")
}

func getVars(namespace, image, replicas string) map[string]string {
	return map[string]string{
		"IMAGE":     image,
		"NAMESPACE": namespace,
		"REPLICAS":  replicas,
	}
}
