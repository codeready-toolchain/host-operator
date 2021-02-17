package registrationservice

import (
	"context"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/test"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCreateOrUpdateResources(t *testing.T) {
	// given
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	cl := NewFakeClient(t)
	config, err := configuration.LoadConfig(cl)
	require.NoError(t, err)

	t.Run("create with default values", func(t *testing.T) {
		// given
		cl := NewFakeClient(t)

		// when
		err = CreateOrUpdateResources(cl, s, HostOperatorNs, config)

		// then
		require.NoError(t, err)
		test.AssertThatRegistrationService(t, "registration-service", cl).
			HasImage("").
			HasEnvironment("").
			HasReplicas("").
			HasAuthConfig("").
			HasAuthLibraryUrl("").
			HasAuthPublicKeysUrl("")

	})

	t.Run("update to RegService with image, environment and auth client values set", func(t *testing.T) {
		// given
		regService := &v1alpha1.RegistrationService{
			ObjectMeta: v1.ObjectMeta{
				Namespace: HostOperatorNs,
				Name:      "registration-service",
			},
			Spec: v1alpha1.RegistrationServiceSpec{
				EnvironmentVariables: map[string]string{
					"IMAGE": "quay.io/rh/registration-service:some-old",
				},
			},
		}
		client := commonclient.NewApplyClient(cl, s)
		_, err := client.ApplyObject(regService)
		require.NoError(t, err)
		restore := SetEnvVarsAndRestore(t,
			Env("REGISTRATION_SERVICE_IMAGE", "quay.io/rh/registration-service:v0.1"),
			Env("REGISTRATION_SERVICE_ENVIRONMENT", "test"),
			Env("REGISTRATION_SERVICE_AUTH_CLIENT_LIBRARY_URL", "url/to/library/location"),
			Env("REGISTRATION_SERVICE_AUTH_CLIENT_CONFIG_RAW", `{"my":"cool-config"}`),
			Env("REGISTRATION_SERVICE_AUTH_CLIENT_PUBLIC_KEYS_URL", "url/to/public/key/location"))
		defer restore()

		// when
		err = CreateOrUpdateResources(cl, s, HostOperatorNs, config)

		// then
		require.NoError(t, err)
		test.AssertThatRegistrationService(t, "registration-service", cl).
			HasImage("quay.io/rh/registration-service:v0.1").
			HasEnvironment("test").
			HasReplicas("").
			HasAuthConfig(`{"my":"cool-config"}`).
			HasAuthLibraryUrl("url/to/library/location").
			HasAuthPublicKeysUrl("url/to/public/key/location")
	})

	t.Run("when creation fails then should return error", func(t *testing.T) {
		// given
		cl := NewFakeClient(t)
		cl.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
			return fmt.Errorf("creation failed")
		}

		// when
		err = CreateOrUpdateResources(cl, s, HostOperatorNs, config)

		// then
		require.Error(t, err)
	})
}
