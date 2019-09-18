package templates_test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/templates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplates(t *testing.T) {

	t.Run("basic", func(t *testing.T) {
		// when
		asset, err := templates.Asset("basic.yml")
		// then
		require.NoError(t, err)
		assert.NotEmpty(t, asset)
	})

	t.Run("advanced", func(t *testing.T) {
		// when
		asset, err := templates.Asset("advanced.yml")
		// then
		require.NoError(t, err)
		assert.NotEmpty(t, asset)
	})
}
