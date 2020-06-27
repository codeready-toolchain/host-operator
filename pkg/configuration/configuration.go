// Package configuration is in charge of the validation and extraction of all
// the configuration details from a configuration file or environment variables.
package configuration

import (
	"context"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"

	errs "github.com/pkg/errors"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// VarDurationBeforeChangeRequestDeletion specificies the duration before a change request is deleted
	VarDurationBeforeChangeRequestDeletion = "duration.before.change.request.deletion"

	// ToolchainConfigMapUserApprovalPolicy is a key for a user approval policy that should be used
	ToolchainConfigMapUserApprovalPolicy = "user-approval-policy"

	// ToolchainConfigMapName specifies a name of a ConfigMap that keeps toolchain configuration
	ToolchainConfigMapName = "toolchain-saas-config"

	// UserApprovalPolicyManual defines that the new users should be approved manually
	UserApprovalPolicyManual = "manual"

	// UserApprovalPolicyAutomatic defines that the new users should be approved automatically
	UserApprovalPolicyAutomatic = "automatic"

	// NotificationDeliveryServiceMock is the notification delivery service to use for testing
	NotificationDeliveryServiceMock = "mock"

	// NotificationDeliveryServiceMailgun is the notification delivery service to use during production
	NotificationDeliveryServiceMailgun = "mailgun"

	varNotificationDeliveryService = "notification.delivery.service"

	// varDurationBeforeNotificationDeletion specifies the duration before a notification will be deleted
	varDurationBeforeNotificationDeletion = "duration.before.notification.deletion"

	// varHostOperatorMailgunDomain specifies the host operator mailgun domain
	varHostOperatorMailgunDomain = "mailgun.domain"

	// varHostOperatorMailgunAPIKey specifies the host operator mailgun api key
	varHostOperatorMailgunAPIKey = "mailgun.api.key"

	// mailgunDomain defines mailgun domain key
	mailgunDomain = "mailgun-domain"

	// mailgunAPIKey defines mailgun api key
	mailgunAPIKey = "mailgun-api-key"

	// defaultNamesapce specifies  the default k8s namespace to use
	defaultNamesapce = "toolchain-host-operator"
)

// Config encapsulates the Viper configuration registry which stores the
// configuration data in-memory.
type Config struct {
	host *viper.Viper
}

// New creates a configuration reader object using a configurable configuration
// file path. If the provided config file path is empty, a default configuration
// will be created.
func New(configFilePath string) (*Config, error) {
	c := initConfig()
	if configFilePath != "" {
		c.host.SetConfigType("yaml")
		c.host.SetConfigFile(configFilePath)
		err := c.host.ReadInConfig() // Find and read the config file
		if err != nil {              // Handle errors reading the config file.
			return nil, errs.Wrap(err, "failed to read config file")
		}
	}
	return c, nil
}

// initConfig creates an initial, empty configuration.
func initConfig() *Config {
	c := Config{
		host: viper.New(),
	}
	c.host.SetEnvPrefix(HostEnvPrefix)
	c.host.AutomaticEnv()
	c.host.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	c.host.SetTypeByDefaultValue(true)
	c.setConfigDefaults()
	return &c
}

func LoadConfig(cl client.Client) *Config {
	getHostOperatorSecret(cl)
	return initConfig()
}

// getHostOperatorSecret retrieves the host operator secret
func getHostOperatorSecret(cl client.Client) error {
	// get the secret name
	secretName := os.Getenv("HOST_OPERATOR_SECRET_NAME")
	if secretName == "" {
		return nil
	}

	// get the secret
	secret := &v1.Secret{}
	namespacedName := types.NamespacedName{Namespace: defaultNamesapce, Name: secretName}
	err := client.Client.Get(cl, context.TODO(), namespacedName, secret)
	if err != nil {
		return err
	}

	// get the domain and domain api key
	mailgunDomain := secret.Data[mailgunDomain]
	mailgunApiKey := secret.Data[mailgunAPIKey]

	// set the domain and api key env vars
	os.Setenv("HOST_OPERATOR_MAILGUN_DOMAIN", string(mailgunDomain))
	os.Setenv("HOST_OPERATOR_MAILGUN_API_KEY", string(mailgunApiKey))

	return nil
}

func (c *Config) setConfigDefaults() {
	c.host.SetTypeByDefaultValue(true)
	c.host.SetDefault(VarDurationBeforeChangeRequestDeletion, "24h")
	c.host.SetDefault(varNotificationDeliveryService, NotificationDeliveryServiceMock)
	c.host.SetDefault(varDurationBeforeNotificationDeletion, "24h")
}

// GetDurationBeforeChangeRequestDeletion returns the timeout before a complete TierChangeRequest will be deleted.
func (c *Config) GetDurationBeforeChangeRequestDeletion() time.Duration {
	return c.host.GetDuration(VarDurationBeforeChangeRequestDeletion)
}

// GetNotificationDeliveryService returns the name of the notification delivery service to use for delivering user notifications
func (c *Config) GetNotificationDeliveryService() string {
	return c.host.GetString(varNotificationDeliveryService)
}

// GetDurationBeforeNotificationDeletion returns the timeout before a delivered notification will be deleted.
func (c *Config) GetDurationBeforeNotificationDeletion() time.Duration {
	return c.host.GetDuration(varDurationBeforeNotificationDeletion)
}

// GetHostOperatorMailgunDomain returns the host operator mailgun domain
func (c *Config) GetHostOperatorMailgunDomain() string {
	return c.host.GetString(varHostOperatorMailgunDomain)
}

// GetHostOperatorMailgunAPIKey returns the host operator mailgun domain
func (c *Config) GetHostOperatorMailAPIKey() string {
	return c.host.GetString(varHostOperatorMailgunAPIKey)
}

// GetAllRegistrationServiceParameters returns the map with key-values pairs of parameters that have REGISTRATION_SERVICE prefix
func (c *Config) GetAllRegistrationServiceParameters() map[string]string {
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
