package configuration_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getDefaultConfiguration returns a configuration registry without anything but
// defaults set. Remember that environment variables can overwrite defaults, so
// please ensure to properly unset envionment variables using
// UnsetEnvVarAndRestore().
func getDefaultConfiguration(t *testing.T) *configuration.Registry {
	config, err := configuration.New("")
	require.NoError(t, err)
	return config
}

// getFileConfiguration returns a configuration based on defaults, the given
// file content and overwrites by environment variables. As with
// getDefaultConfiguration() remember that environment variables can overwrite
// defaults, so please ensure to properly unset envionment variables using
// UnsetEnvVarAndRestore().
func getFileConfiguration(t *testing.T, content string) *configuration.Registry {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "configFile-")
	require.NoError(t, err)
	defer func() {
		err := os.Remove(tmpFile.Name())
		require.NoError(t, err)
	}()
	_, err = tmpFile.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	config, err := configuration.New(tmpFile.Name())
	require.NoError(t, err)
	return config
}

func TestNew(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		reg, err := configuration.New("")
		require.NoError(t, err)
		require.NotNil(t, reg)
	})
	t.Run("non existing file path", func(t *testing.T) {
		u, err := uuid.NewV4()
		require.NoError(t, err)
		reg, err := configuration.New(u.String())
		require.Error(t, err)
		require.Nil(t, reg)
	})
}

func TestGetImage(t *testing.T) {
	key := configuration.RegServiceEnvPrefix + "_" + "IMAGE"
	resetFunc := test.UnsetEnvVarAndRestore(t, key)
	defer resetFunc()

	t.Run("default", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		config := getDefaultConfiguration(t)
		assert.Equal(t, "", config.GetRegServiceImage())
	})

	t.Run("file", func(t *testing.T) {
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		config := getFileConfiguration(t, `image: "`+newVal+`"`)
		assert.Equal(t, newVal, config.GetRegServiceImage())
	})

	t.Run("env overwrite", func(t *testing.T) {
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		restore := test.SetEnvVarAndRestore(t, key, newVal)
		defer restore()

		restore = test.SetEnvVarAndRestore(t, configuration.RegServiceEnvPrefix+"_"+"ANY_CONFIG", newVal)
		defer restore()
		config := getDefaultConfiguration(t)
		assert.Equal(t, newVal, config.GetRegServiceImage())
	})
}

func TestGetDynamicKeys(t *testing.T) {
	firstKey := configuration.RegServiceEnvPrefix + "_" + "FIRST_KEY"
	secondKey := configuration.RegServiceEnvPrefix + "_" + "SECOND_KEY"

	t.Run("default", func(t *testing.T) {
		config := getDefaultConfiguration(t)
		assert.Empty(t, config.GetDynamicallyAddedParameters())
	})

	t.Run("file with empty list ", func(t *testing.T) {
		config := getFileConfiguration(t, `ANYTHING: "SOMETHING"`)
		assert.Empty(t, config.GetDynamicallyAddedParameters())
	})

	t.Run("env overwrite", func(t *testing.T) {
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		u2, err := uuid.NewV4()
		require.NoError(t, err)
		newVal2 := u2.String()
		//todo fix this
		restore := test.SetEnvVarsAndRestore(t,
			test.Env(firstKey, newVal),
			test.Env(secondKey, newVal2))
		defer restore()
		config := getDefaultConfiguration(t)
		require.Len(t, config.GetDynamicallyAddedParameters(), 2)
		assert.Equal(t, newVal, config.GetDynamicallyAddedParameters()[firstKey])
		assert.Equal(t, newVal2, config.GetDynamicallyAddedParameters()[secondKey])
	})
}
