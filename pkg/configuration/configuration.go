// Package configuration is in charge of the validation and extraction of all
// the configuration details from a configuration file or environment variables.
package configuration

import (
	"os"
	"strings"

	errs "github.com/pkg/errors"
	"github.com/spf13/viper"
)

// prefixes
const (
	// HostEnvPrefix will be used for host environment variable name prefixing.
	HostEnvPrefix = "HOST_OPERATOR"

	// RegServiceEnvPrefix will be used for registration service environment variable name prefixing.
	RegServiceEnvPrefix = "REGISTRATION_SERVICE"
)

// host-operator constants
const (
	// ToolchainConfigMapName specifies a name of a ConfigMap that keeps toolchain configuration
	ToolchainConfigMapName = "toolchain-saas-config"

	// ToolchainConfigMapUserApprovalPolicy is a key for a user approval policy that should be used
	ToolchainConfigMapUserApprovalPolicy = "user-approval-policy"

	// UserApprovalPolicyManual defines that the new users should be approved manually
	UserApprovalPolicyManual = "manual"

	// UserApprovalPolicyAutomatic defines that the new users should be approved automatically
	UserApprovalPolicyAutomatic = "automatic"
)

// Registry encapsulates the Viper configuration registry which stores the
// configuration data in-memory.
type Registry struct {
	host *viper.Viper
}

// CreateEmptyRegistry creates an initial, empty registry.
func CreateEmptyRegistry() *Registry {
	c := Registry{
		host: viper.New(),
	}
	c.host.SetEnvPrefix(HostEnvPrefix)
	c.host.AutomaticEnv()
	c.host.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	c.host.SetTypeByDefaultValue(true)
	c.setConfigDefaults()
	return &c
}

// New creates a configuration reader object using a configurable configuration
// file path. If the provided config file path is empty, a default configuration
// will be created.
func New(configFilePath string) (*Registry, error) {
	c := CreateEmptyRegistry()
	if configFilePath != "" {
		c.host.SetConfigType("yaml")
		c.host.SetConfigFile(configFilePath)
		err := c.host.ReadInConfig() // Find and read the config file
		if err != nil { // Handle errors reading the config file.
			return nil, errs.Wrap(err, "failed to read config file")
		}
	}
	return c, nil
}

func (c *Registry) setConfigDefaults() {
	c.host.SetTypeByDefaultValue(true)
}

// GetAllRegistrationServiceParameters returns the map with key-values pairs of parameters that have REGISTRATION_SERVICE prefix
func (c *Registry) GetAllRegistrationServiceParameters() map[string]string {
	vars := map[string]string{}

	for _, env := range os.Environ() {
		keyValue := strings.SplitN(env, "=", 2)
		if len(keyValue) < 2 {
			continue
		}
		if strings.HasPrefix(keyValue[0], RegServiceEnvPrefix+"_") {
			vars[keyValue[0]] = keyValue[1]
		}
	}
	return vars
}
