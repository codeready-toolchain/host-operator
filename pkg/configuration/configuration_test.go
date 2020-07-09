package configuration_test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/gofrs/uuid"
	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getDefaultConfiguration returns a configuration registry without anything but
// defaults set. Remember that environment variables can overwrite defaults, so
// please ensure to properly unset envionment variables using
// UnsetEnvVarAndRestore().
func getDefaultConfiguration(t *testing.T) *configuration.Config {
	config, err := configuration.LoadConfig(test.NewFakeClient(t))
	require.NoError(t, err)
	return config
}

func TestNew(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		require.NotNil(t, getDefaultConfiguration(t))
	})
}

func TestGetAllRegistrationServiceParameters(t *testing.T) {
	firstKey := configuration.RegServiceEnvPrefix + "_" + "FIRST_KEY"
	secondKey := configuration.RegServiceEnvPrefix + "_" + "SECOND_KEY"
	thirdKey := configuration.RegServiceEnvPrefix + "_" + "THIRD_KEY"

	t.Run("default", func(t *testing.T) {
		config := getDefaultConfiguration(t)
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
		config := getDefaultConfiguration(t)
		require.Len(t, config.GetAllRegistrationServiceParameters(), 3)
		assert.Equal(t, newVal, config.GetAllRegistrationServiceParameters()[firstKey])
		assert.Equal(t, newVal2, config.GetAllRegistrationServiceParameters()[secondKey])
		assert.Empty(t, config.GetAllRegistrationServiceParameters()[thirdKey])
	})
}

func TestLoadFromSecret(t *testing.T) {
	restore := test.SetEnvVarAndRestore(t, "WATCH_NAMESPACE", "toolchain-host-operator")
	defer restore()
	t.Run("default", func(t *testing.T) {
		// when
		config := getDefaultConfiguration(t)

		// then
		assert.Equal(t, "", config.GetMailgunDomain())
		assert.Equal(t, "", config.GetMailgunAPIKey())
	})
	t.Run("env overwrite", func(t *testing.T) {
		// given
		restore := test.SetEnvVarAndRestore(t, "HOST_OPERATOR_SECRET_NAME", "test-secret")
		defer restore()

		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: "toolchain-host-operator",
			},
			Data: map[string][]byte{
				"mailgun-domain":       []byte("test-domain"),
				"mailgun-api-key":      []byte("test-api-key"),
				"mailgun-sender-email": []byte("test-sender-email"),
			},
		}

		cl := test.NewFakeClient(t, secret)

		// when
		config, err := configuration.LoadConfig(cl)

		// then
		require.NoError(t, err)
		assert.Equal(t, "test-domain", config.GetMailgunDomain())
		assert.Equal(t, "test-api-key", config.GetMailgunAPIKey())
		assert.Equal(t, "test-sender-email", config.GetMailgunSenderEmail())
	})

	t.Run("secret not found", func(t *testing.T) {
		// given
		restore := test.SetEnvVarAndRestore(t, "HOST_OPERATOR_SECRET_NAME", "test-secret")
		defer restore()

		cl := test.NewFakeClient(t)

		// when
		config, err := configuration.LoadConfig(cl)

		// then
		require.Error(t, err)
		assert.Equal(t, "secrets \"test-secret\" not found", err.Error())
		assert.Nil(t, config)
	})
}

func TestGetDurationBeforeChangeRequestDeletion(t *testing.T) {
	key := configuration.HostEnvPrefix + "_" + "DURATION_BEFORE_CHANGE_REQUEST_DELETION"
	resetFunc := test.UnsetEnvVarAndRestore(t, key)
	defer resetFunc()

	t.Run("default", func(t *testing.T) {
		resetFunc := test.UnsetEnvVarAndRestore(t, key)
		defer resetFunc()
		config := getDefaultConfiguration(t)
		assert.Equal(t, cast.ToDuration("24h"), config.GetDurationBeforeChangeRequestDeletion())
	})

	t.Run("env overwrite", func(t *testing.T) {
		restore := test.SetEnvVarAndRestore(t, key, "10s")
		defer restore()

		restore = test.SetEnvVarAndRestore(t, configuration.HostEnvPrefix+"_"+"ANY_CONFIG", "20s")
		defer restore()
		config := getDefaultConfiguration(t)
		assert.Equal(t, cast.ToDuration("10s"), config.GetDurationBeforeChangeRequestDeletion())
	})
}
