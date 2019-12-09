package configuration_test

import (
	"fmt"
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

func TestGetRegServiceEnvironment(t *testing.T) {
	key := fmt.Sprintf("%s_ENVIRONMENT", configuration.RegServiceEnvPrefix)
	resetFunc := test.UnsetEnvVarAndRestore(t, key)
	defer resetFunc()

	t.Run("default", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		config := getDefaultConfiguration(t)
		assert.Equal(t, "prod", config.GetRegServiceEnvironment())
	})

	t.Run("file", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		config := getFileConfiguration(t, "environment: TestGetEnvironmentFromConfig")
		assert.Equal(t, "TestGetEnvironmentFromConfig", config.GetRegServiceEnvironment())
	})

	t.Run("env overwrite", func(t *testing.T) {
		err := os.Setenv(key, "TestGetEnvironmentFromEnvVar")
		require.NoError(t, err)
		config := getDefaultConfiguration(t)
		assert.Equal(t, "TestGetEnvironmentFromEnvVar", config.GetRegServiceEnvironment())
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
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
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
		err = os.Setenv(key, newVal)
		require.NoError(t, err)
		config := getDefaultConfiguration(t)
		assert.Equal(t, newVal, config.GetRegServiceImage())
	})
}

func TestGetAuthClientLibraryURL(t *testing.T) {
	key := configuration.RegServiceEnvPrefix + "_" + "AUTH_CLIENT_LIBRARY_URL"
	resetFunc := test.UnsetEnvVarAndRestore(t, key)
	defer resetFunc()

	t.Run("default", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		config := getDefaultConfiguration(t)
		assert.Equal(t, "", config.GetAuthClientLibraryURL())
	})

	t.Run("file", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		config := getFileConfiguration(t, `auth_client.library_url: "`+newVal+`"`)
		assert.Equal(t, newVal, config.GetAuthClientLibraryURL())
	})

	t.Run("env overwrite", func(t *testing.T) {
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		err = os.Setenv(key, newVal)
		require.NoError(t, err)
		config := getDefaultConfiguration(t)
		assert.Equal(t, newVal, config.GetAuthClientLibraryURL())
	})
}

func TestGetAuthClientPublicKeysURL(t *testing.T) {
	key := configuration.RegServiceEnvPrefix + "_" + "AUTH_CLIENT_PUBLIC_KEYS_URL"
	resetFunc := test.UnsetEnvVarAndRestore(t, key)
	defer resetFunc()

	t.Run("default", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		config := getDefaultConfiguration(t)
		assert.Equal(t, "", config.GetAuthClientPublicKeysURL())
	})

	t.Run("file", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		config := getFileConfiguration(t, `auth_client.public_keys_url: "`+newVal+`"`)
		assert.Equal(t, newVal, config.GetAuthClientPublicKeysURL())
	})

	t.Run("env overwrite", func(t *testing.T) {
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		err = os.Setenv(key, newVal)
		require.NoError(t, err)
		config := getDefaultConfiguration(t)
		assert.Equal(t, newVal, config.GetAuthClientPublicKeysURL())
	})
}

func TestGetAuthClientConfigRaw(t *testing.T) {
	key := configuration.RegServiceEnvPrefix + "_" + "AUTH_CLIENT_CONFIG_RAW"

	t.Run("default", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		config := getDefaultConfiguration(t)
		assert.Equal(t, "", config.GetAuthClientConfigAuthRaw())
	})

	t.Run("file", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		config := getFileConfiguration(t, `auth_client.config.raw: "`+newVal+`"`)
		assert.Equal(t, newVal, config.GetAuthClientConfigAuthRaw())
	})

	t.Run("env overwrite", func(t *testing.T) {
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		err = os.Setenv(key, newVal)
		require.NoError(t, err)
		config := getDefaultConfiguration(t)
		assert.Equal(t, newVal, config.GetAuthClientConfigAuthRaw())
	})
}
