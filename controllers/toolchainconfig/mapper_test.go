package toolchainconfig

import (
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestSecretToToolchainConfigMapper(t *testing.T) {
	// when
	secretData := map[string][]byte{
		"mailgunAPIKey": []byte("abc123"),
	}

	t.Run("test secret maps correctly", func(t *testing.T) {
		//given
		secret := test.CreateSecret("test-secret", test.HostOperatorNs, secretData)

		// when
		req := MapSecretToToolchainConfig()(secret)

		// then
		require.Len(t, req, 1)
		require.Equal(t, types.NamespacedName{
			Namespace: test.HostOperatorNs,
			Name:      "config",
		}, req[0].NamespacedName)
	})

	t.Run("a non-secret resource is not mapped", func(t *testing.T) {
		// given
		pod := &corev1.Pod{}

		// when
		req := MapSecretToToolchainConfig()(pod)

		// then
		require.Len(t, req, 0)
	})
}
