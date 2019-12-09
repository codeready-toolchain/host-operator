// Package configuration is in charge of the validation and extraction of all
// the configuration details from a configuration file or environment variables.
package configuration

import (
	"strings"

	errs "github.com/pkg/errors"
	"github.com/spf13/viper"
)

// prefixes
const (
	// HostEnvPrefix will be used for host environment variable name prefixing.
	HostEnvPrefix = "HOST"

	// RegServiceEnvPrefix will be used for registration service environment variable name prefixing.
	RegServiceEnvPrefix = "REGISTRATION_SERVICE"
)

// registration-service constants
const (
	// varImage specifies registration service image to be used for deployment
	varImage = "image"

	// varEnvironment specifies registration service environment such as prod, stage, unit-tests, e2e-tests, dev, etc
	varEnvironment = "environment"
	// DefaultEnvironment is the default registration service environment
	DefaultEnvironment = "prod"

	// varAuthClientLibraryURL identifies the auth library location
	varAuthClientLibraryURL = "auth_client.library_url"

	// varAuthClientConfigRaw contains the auth config
	varAuthClientConfigRaw = "auth_client.config.raw"

	// varAuthClientPublicKeysURL identifies the public keys location
	varAuthClientPublicKeysURL = "auth_client.public_keys_url"
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
	host       *viper.Viper
	regService *viper.Viper
}

// CreateEmptyRegistry creates an initial, empty registry.
func CreateEmptyRegistry() *Registry {
	c := Registry{
		host:       viper.New(),
		regService: viper.New(),
	}
	c.host.SetEnvPrefix(HostEnvPrefix)
	c.regService.SetEnvPrefix(RegServiceEnvPrefix)
	for _, v := range []*viper.Viper{c.host, c.regService} {
		v.AutomaticEnv()
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.SetTypeByDefaultValue(true)
	}
	c.setConfigDefaults()
	return &c
}

// New creates a configuration reader object using a configurable configuration
// file path. If the provided config file path is empty, a default configuration
// will be created.
func New(configFilePath string) (*Registry, error) {
	c := CreateEmptyRegistry()
	if configFilePath != "" {
		for _, reg := range []*viper.Viper{c.host, c.regService} {
			reg.SetConfigType("yaml")
			reg.SetConfigFile(configFilePath)
			err := reg.ReadInConfig() // Find and read the config file
			if err != nil { // Handle errors reading the config file.
				return nil, errs.Wrap(err, "failed to read config file")
			}
		}
	}
	return c, nil
}

// GetViperInstance returns the underlying Viper instance.
func (c *Registry) GetViperInstance() *viper.Viper {
	return c.host
}

func (c *Registry) setConfigDefaults() {
	c.host.SetTypeByDefaultValue(true)
	c.regService.SetTypeByDefaultValue(true)

	c.regService.SetDefault(varEnvironment, DefaultEnvironment)
}

// GetRegServiceImage returns the registration service image.
func (c *Registry) GetRegServiceImage() string {
	return c.regService.GetString(varImage)
}

// GetRegServiceEnvironment returns the registration service environment such as prod, stage, unit-tests, e2e-tests, dev, etc
func (c *Registry) GetRegServiceEnvironment() string {
	return c.regService.GetString(varEnvironment)
}

// GetAuthClientLibraryURL returns the auth library location
func (c *Registry) GetAuthClientLibraryURL() string {
	return c.regService.GetString(varAuthClientLibraryURL)
}

// GetAuthClientConfigAuthRaw returns the auth config config
func (c *Registry) GetAuthClientConfigAuthRaw() string {
	return c.regService.GetString(varAuthClientConfigRaw)
}

// GetAuthClientPublicKeysURL returns the public keys URL
func (c *Registry) GetAuthClientPublicKeysURL() string {
	return c.regService.GetString(varAuthClientPublicKeysURL)
}
