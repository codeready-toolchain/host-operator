package toolchainstatus

import (
	"context"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	. "github.com/codeready-toolchain/host-operator/test"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCreateOrUpdateResources(t *testing.T) {
	// given
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	name := toolchainconfig.ToolchainStatusName

	t.Run("creation", func(t *testing.T) {
		// given
		cl := commontest.NewFakeClient(t)

		// when
		err = CreateOrUpdateResources(context.TODO(), cl, commontest.HostOperatorNs, name)

		// then
		require.NoError(t, err)

		// check that the MemberStatus exists now
		AssertThatToolchainStatus(t, commontest.HostOperatorNs, name, cl).Exists()
	})

	t.Run("should return an error if creation fails ", func(t *testing.T) {
		// given
		cl := commontest.NewFakeClient(t)
		cl.MockPatch = func(_ context.Context, _ runtimeclient.Object, _ runtimeclient.Patch, _ ...runtimeclient.PatchOption) error {
			return fmt.Errorf("patch failed")
		}

		// when
		err = CreateOrUpdateResources(context.TODO(), cl, commontest.HostOperatorNs, name)

		// then
		require.Error(t, err)
		require.Equal(t, "unable to patch 'toolchain.dev.openshift.com/v1alpha1, Kind=ToolchainStatus' called 'toolchain-status' in namespace 'toolchain-host-operator': patch failed", err.Error())
	})
}
