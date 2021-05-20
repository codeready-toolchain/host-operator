package registrationservice

import (
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/template"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDeploymentAssetContainsAllNecessaryInformation(t *testing.T) {
	// given
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	deploymentTemplate, err := getDeploymentTemplate(s)
	require.NoError(t, err)
	vars := getVars(newRegistrationService("my-namespace", "quay.io/cr-t/registration-service:123", "dev", 10))
	processor := template.NewProcessor(s)

	// when
	toolchainObjects, err := processor.Process(deploymentTemplate, vars)

	// then
	require.NoError(t, err)
	deploymentFound := false
	saFound := false
	roleFound := false
	roleBindingFound := false
	for _, toolchainObject := range toolchainObjects {
		assert.Equal(t, "my-namespace", toolchainObject.GetNamespace())
		fmt.Println(toolchainObject.GetGvk())
		fmt.Println(appsv1.SchemeGroupVersion.WithKind("Deployment"))

		switch toolchainObject.GetGvk() {
		case appsv1.SchemeGroupVersion.WithKind("Deployment"):
			deploymentFound = true
			deployment := fmt.Sprintf("%+v", toolchainObject.GetRuntimeObject())
			assert.Contains(t, deployment, "replicas:10")
			assert.Contains(t, deployment, "image:quay.io/cr-t/registration-service:123")

		case corev1.SchemeGroupVersion.WithKind("ServiceAccount"):
			saFound = true
		case rbacv1.SchemeGroupVersion.WithKind("Role"):
			roleFound = true
		case rbacv1.SchemeGroupVersion.WithKind("RoleBinding"):
			roleBindingFound = true

		}
	}
	assert.True(t, deploymentFound, "a Deployment wasn't found")
	assert.True(t, saFound, "a ServiceAccount wasn't found")
	assert.True(t, roleFound, "a Role wasn't found")
	assert.True(t, roleBindingFound, "a RoleBinding wasn't found")
}

func newRegistrationService(namespace, image, env string, replicas int) *v1alpha1.RegistrationService {
	return &v1alpha1.RegistrationService{
		TypeMeta: metav1.TypeMeta{
			Kind: "RegistrationService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "registration-service",
		},
		Spec: v1alpha1.RegistrationServiceSpec{
			EnvironmentVariables: map[string]string{
				"IMAGE":       image,
				"ENVIRONMENT": env,
				"REPLICAS":    fmt.Sprintf("%d", replicas),
			},
		},
	}
}
