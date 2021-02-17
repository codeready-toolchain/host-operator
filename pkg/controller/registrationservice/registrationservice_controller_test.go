package registrationservice

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/codeready-toolchain/api/pkg/apis"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	. "github.com/codeready-toolchain/host-operator/test"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/template"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	tmplv1 "github.com/openshift/api/template/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileRegistrationService(t *testing.T) {
	// given
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	codecFactory := serializer.NewCodecFactory(s)
	decoder := codecFactory.UniversalDeserializer()

	tmpl := getDecodedTemplate(t, decoder)
	reqService := newRegistrationService(test.HostOperatorNs, imageDef, "dev", 1)
	p := template.NewProcessor(s)
	objs, err := p.Process(tmpl, getVars(reqService))
	require.NoError(t, err)

	t.Run("create both objects", func(t *testing.T) {
		// given
		service, request := prepareServiceAndRequest(t, s, decoder, reqService)

		// when
		result, err := service.Reconcile(request)

		// then
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.Equal(t, time.Second, result.RequeueAfter)
		AssertThatServiceAccount(t, test.HostOperatorNs, "registration-service", service.client).HasOwner(reqService)
		AssertThatConfigMap(t, test.HostOperatorNs, "registration-service", service.client).HasOwner(reqService).
			HasData(map[string]string{
				"reg-service-image": imageDef,
				"reg-service-env":   "dev",
			})
		AssertThatRegistrationService(t, "registration-service", service.client).
			HasConditions(toBeNotReady("Deploying", "updated resources: [ServiceAccount: registration-service ConfigMap: registration-service]"))
	})

	t.Run("reconcile when both objects are present and don't update nor create anything", func(t *testing.T) {
		// given
		service, request := prepareServiceAndRequest(t, s, decoder, reqService)
		cclient := commonclient.NewApplyClient(service.client, service.scheme)
		_, err := cclient.ApplyObject(objs[0].GetRuntimeObject().DeepCopyObject())
		require.NoError(t, err)
		_, err = cclient.ApplyObject(objs[1].GetRuntimeObject().DeepCopyObject())
		require.NoError(t, err)
		fakeClient := service.client.(*test.FakeClient)
		fakeClient.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
			return fmt.Errorf("create shouldn't be called")
		}
		fakeClient.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
			return fmt.Errorf("update shouldn't be called")
		}

		// when
		result, err := service.Reconcile(request)

		// then
		require.NoError(t, err)
		assert.False(t, result.Requeue)
		AssertThatServiceAccount(t, test.HostOperatorNs, "registration-service", service.client).Exists()
		AssertThatConfigMap(t, test.HostOperatorNs, "registration-service", service.client).HasData(map[string]string{
			"reg-service-image": imageDef,
			"reg-service-env":   "dev",
		})

		AssertThatRegistrationService(t, "registration-service", service.client).
			HasConditions(toBeDeployed())
	})

	t.Run("change ConfigMap object & don't specify environment so it uses the default one", func(t *testing.T) {
		// given
		service, request := prepareServiceAndRequest(t, s, decoder)
		client := commonclient.NewApplyClient(service.client, service.scheme)
		_, err := client.ApplyObject(objs[0].GetRuntimeObject().DeepCopyObject())
		require.NoError(t, err)
		_, err = client.ApplyObject(objs[1].GetRuntimeObject().DeepCopyObject())
		require.NoError(t, err)
		reqService := newRegistrationService(test.HostOperatorNs, "quay.io/rh/registration-service:v0.1", "", 1)
		_, err = client.ApplyObject(reqService)
		require.NoError(t, err)

		// when
		result, err := service.Reconcile(request)

		// then
		require.NoError(t, err)
		assert.True(t, result.Requeue)
		assert.Equal(t, time.Second, result.RequeueAfter)
		AssertThatServiceAccount(t, test.HostOperatorNs, "registration-service", service.client).Exists()
		AssertThatConfigMap(t, test.HostOperatorNs, "registration-service", service.client).Exists().HasData(map[string]string{
			"reg-service-image": "quay.io/rh/registration-service:v0.1",
			"reg-service-env":   "prod",
		})

		AssertThatRegistrationService(t, "registration-service", service.client).
			HasConditions(toBeNotReady("Deploying", "updated resources: [ConfigMap: registration-service]"))
	})

	t.Run("when cannot create, then it should set appropriate condition", func(t *testing.T) {
		// given
		service, request := prepareServiceAndRequest(t, s, decoder, reqService)
		fakeClient := service.client.(*test.FakeClient)
		fakeClient.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
			return fmt.Errorf("creation failed")
		}

		// when
		_, err := service.Reconcile(request)

		// then
		require.Error(t, err)
		AssertThatRegistrationService(t, "registration-service", service.client).
			HasConditions(toBeNotReady("DeployingFailed", "unable to create resource of kind: ServiceAccount, version: v1: creation failed"))
	})

	t.Run("status update of the RegistrationService failed", func(t *testing.T) {
		// given
		service, _ := prepareServiceAndRequest(t, s, decoder, reqService)
		statusUpdater := func(regServ *v1alpha1.RegistrationService, message string) error {
			return fmt.Errorf("unable to update status")
		}

		// when
		err := service.wrapErrorWithStatusUpdate(log, reqService, statusUpdater,
			errors.NewBadRequest("oopsy woopsy"), "template deployment failed")

		// then
		require.Error(t, err)
		assert.Equal(t, "template deployment failed: oopsy woopsy", err.Error())
	})
}

func TestGetVarsWhenAuthClientIsNotSpecified(t *testing.T) {
	// given
	reqService := newRegistrationService(test.HostOperatorNs, imageDef, "dev", 1)

	// when
	vars := getVars(reqService)

	// then
	assert.Len(t, vars, 4)
	assert.Equal(t, test.HostOperatorNs, vars["NAMESPACE"])
	assert.Equal(t, imageDef, vars["IMAGE"])
	assert.Equal(t, "dev", vars["ENVIRONMENT"])
	assert.Equal(t, "1", vars["REPLICAS"])
}

func TestGetVarsWhenAuthClientIsSpecifiedButNotEnv(t *testing.T) {
	// given
	regService := newRegistrationService("host-operator", imageDef, "", 1)

	regService.Spec.EnvironmentVariables["AUTH_CLIENT_LIBRARY_URL"] = "location/of/library"
	regService.Spec.EnvironmentVariables["AUTH_CLIENT_PUBLIC_KEYS_URL"] = "location/of/public/key"
	regService.Spec.EnvironmentVariables["AUTH_CLIENT_CONFIG_RAW"] = `{"my":"cool-config"}`

	// when
	vars := getVars(regService)

	// then
	assert.Len(t, vars, 6)
	assert.Equal(t, "host-operator", vars["NAMESPACE"])
	assert.Equal(t, imageDef, vars["IMAGE"])
	assert.Equal(t, "location/of/library", vars["AUTH_CLIENT_LIBRARY_URL"])
	assert.Equal(t, `{"my":"cool-config"}`, vars["AUTH_CLIENT_CONFIG_RAW"])
	assert.Equal(t, "location/of/public/key", vars["AUTH_CLIENT_PUBLIC_KEYS_URL"])
}

func prepareServiceAndRequest(t *testing.T, s *runtime.Scheme, decoder runtime.Decoder, initObjs ...runtime.Object) (*ReconcileRegistrationService, reconcile.Request) {
	tmpl := getDecodedTemplate(t, decoder)

	service := &ReconcileRegistrationService{
		client:             test.NewFakeClient(t, initObjs...),
		scheme:             s,
		regServiceTemplate: tmpl,
	}
	return service, reconcile.Request{NamespacedName: test.NamespacedName(test.HostOperatorNs, "registration-service")}
}

func getDecodedTemplate(t *testing.T, decoder runtime.Decoder) *tmplv1.Template {
	testTemplate := test.CreateTemplate(test.WithObjects(test.ServiceAccount, configMap), test.WithParams(test.NamespaceParam, registrationServiceParam))
	tmpl, err := test.DecodeTemplate(decoder, testTemplate)
	require.NoError(t, err)
	return tmpl
}

const (
	imageDef = "quay.io/codeready-toolchain/registration-service:1574865601"

	registrationServiceParam test.TemplateParam = `
- name: IMAGE
  value: quay.io/openshiftio/codeready-toolchain/registration-service:latest
- name: REPLICAS
  value: '3'
- name: ENVIRONMENT
  value: 'prod'`

	configMap test.TemplateObject = `
- kind: ConfigMap
  apiVersion: v1
  metadata:
    labels:
      provider: codeready-toolchain
    name: registration-service
    namespace: ${NAMESPACE}
  type: Opaque
  data:
    reg-service-image: ${IMAGE}
    reg-service-env: ${ENVIRONMENT}`
)
