package toolchaincluster

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/deploy"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestLoadObjectsFromTemplates(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		// when
		objects, err := LoadObjectsFromTemplates(&deploy.ToolchainClusterTemplateFS, &TemplateVariables{Namespace: test.HostOperatorNs})
		// then
		require.NoError(t, err)
		require.NotNil(t, objects)
		require.True(t, len(objects) > 1, "invalid number of expected objects")
	})

	t.Run("error - when variables are not provided", func(t *testing.T) {
		// when
		objects, err := LoadObjectsFromTemplates(&deploy.ToolchainClusterTemplateFS, nil)
		// then
		require.Error(t, err)
		require.Nil(t, objects)
	})
}
