package registrationservice

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/test"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCreateOrUpdateResources(t *testing.T) {
	// given
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("create with default values", func(t *testing.T) {
		restoreWatchNamespace := SetEnvVarAndRestore(t, commonconfig.WatchNamespaceEnvVar, HostOperatorNs)
		defer restoreWatchNamespace()
		// given
		cl := NewFakeClient(t)

		// when
		err = CreateOrUpdateResources(cl, s, HostOperatorNs)

		// then
		require.NoError(t, err)
		test.AssertThatRegistrationService(t, "registration-service", cl).
			HasImage("").HasReplicas("3")

	})

	t.Run("update to RegService with image value and replicas set", func(t *testing.T) {
		restoreWatchNamespace := SetEnvVarAndRestore(t, commonconfig.WatchNamespaceEnvVar, HostOperatorNs)
		defer restoreWatchNamespace()
		// given
		regService := &toolchainv1alpha1.RegistrationService{
			ObjectMeta: v1.ObjectMeta{
				Namespace: HostOperatorNs,
				Name:      "registration-service",
			},
			Spec: toolchainv1alpha1.RegistrationServiceSpec{
				EnvironmentVariables: map[string]string{
					"IMAGE": "quay.io/rh/registration-service:some-old",
				},
			},
		}

		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.RegistrationService().
			Replicas(int32(2)))

		cl := NewFakeClient(t, cfg)
		client := commonclient.NewApplyClient(cl, s)
		_, err := client.ApplyObject(regService)
		require.NoError(t, err)
		restore := SetEnvVarsAndRestore(t,
			Env("REGISTRATION_SERVICE_IMAGE", "quay.io/rh/registration-service:v0.1"))
		defer restore()

		// when
		err = CreateOrUpdateResources(cl, s, HostOperatorNs)

		// then
		require.NoError(t, err)
		test.AssertThatRegistrationService(t, "registration-service", cl).
			HasImage("quay.io/rh/registration-service:v0.1").HasReplicas("2")
	})

	t.Run("when creation fails then should return error", func(t *testing.T) {
		restoreWatchNamespace := SetEnvVarAndRestore(t, commonconfig.WatchNamespaceEnvVar, HostOperatorNs)
		defer restoreWatchNamespace()
		// given
		cl := NewFakeClient(t)
		cl.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			return fmt.Errorf("creation failed")
		}

		// when
		err = CreateOrUpdateResources(cl, s, HostOperatorNs)

		// then
		require.Error(t, err)
	})

	t.Run("when WATCH_NAMESPACE not set then should return error", func(t *testing.T) {
		// given
		cl := NewFakeClient(t)
		cl.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			return fmt.Errorf("creation failed")
		}

		// when
		err = CreateOrUpdateResources(cl, s, HostOperatorNs)

		// then
		require.EqualError(t, err, "failed to get watch namespace: WATCH_NAMESPACE must be set")
	})
}
