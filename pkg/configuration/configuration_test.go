package configuration_test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/spf13/cast"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getDefaultConfiguration returns a configuration registry without anything but
// defaults set. Remember that environment variables can overwrite defaults, so
// please ensure to properly unset envionment variables using
// UnsetEnvVarAndRestore().
func getDefaultConfiguration() *configuration.Config {
	return configuration.LoadConfig()
}

func TestNew(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		require.NotNil(t, getDefaultConfiguration())
	})
}

func TestGetAllRegistrationServiceParameters(t *testing.T) {
	firstKey := configuration.RegServiceEnvPrefix + "_" + "FIRST_KEY"
	secondKey := configuration.RegServiceEnvPrefix + "_" + "SECOND_KEY"
	thirdKey := configuration.RegServiceEnvPrefix + "_" + "THIRD_KEY"

	t.Run("default", func(t *testing.T) {
		config := getDefaultConfiguration()
		assert.Empty(t, config.GetAllRegistrationServiceParameters())
	})

	t.Run("env overwrite", func(t *testing.T) {
		u, err := uuid.NewV4()
		require.NoError(t, err)
		newVal := u.String()
		newVal2 := "foo=bar=baz"

		restore := test.SetEnvVarsAndRestore(t,
			test.Env(firstKey, newVal),
			test.Env(secondKey, newVal2),
			test.Env(thirdKey, ""))
		defer restore()
		config := getDefaultConfiguration()
		require.Len(t, config.GetAllRegistrationServiceParameters(), 3)
		assert.Equal(t, newVal, config.GetAllRegistrationServiceParameters()[firstKey])
		assert.Equal(t, newVal2, config.GetAllRegistrationServiceParameters()[secondKey])
		assert.Empty(t, config.GetAllRegistrationServiceParameters()[thirdKey])
	})
}

func TestGetDurationBeforeChangeRequestDeletion(t *testing.T) {
	key := configuration.HostEnvPrefix + "_" + "DURATION_BEFORE_CHANGE_REQUEST_DELETION"
	resetFunc := test.UnsetEnvVarAndRestore(t, key)
	defer resetFunc()

	t.Run("default", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		config := getDefaultConfiguration()
		assert.Equal(t, cast.ToDuration("24h"), config.GetDurationBeforeChangeRequestDeletion())
	})

	t.Run("env overwrite", func(t *testing.T) {
		restore := test.SetEnvVarAndRestore(t, key, "10s")
		defer restore()

		restore = test.SetEnvVarAndRestore(t, configuration.HostEnvPrefix+"_"+"ANY_CONFIG", "20s")
		defer restore()
		config := getDefaultConfiguration()
		assert.Equal(t, cast.ToDuration("10s"), config.GetDurationBeforeChangeRequestDeletion())
	})
}
