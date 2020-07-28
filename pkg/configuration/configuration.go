// Package configuration is in charge of the validation and extraction of all
// the configuration details from a configuration file or environment variables.
package configuration

import (
	"os"
	"strings"
	"time"

	"github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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
	// ToolchainStatusName specifies the name of the toolchain status resource that provides information about the toolchain components in this cluster
	ToolchainStatusName = "toolchain.status"

	// DefaultToolchainStatusName the default name for the toolchain status resource created during initialization of the operator
	DefaultToolchainStatusName = "toolchain-status"

	// VarDurationBeforeChangeRequestDeletion specifies the duration before a ChangeTierRequest resource is deleted
	VarDurationBeforeChangeRequestDeletion = "duration.before.change.request.deletion" // TODO: add missing `tier` in `change.tier.request`?

	// defaultDurationBeforeChangeTierRequestDeletion is the time before a ChangeTierRequest resource is deleted
	defaultDurationBeforeChangeTierRequestDeletion = "24h"

	// varTemplateUpdateRequestMaxPoolSize specifies the maximum number of concurrent TemplateUpdateRequests when updating MasterUserRecords
	varTemplateUpdateRequestMaxPoolSize = "template.update.request.max.poolsize"

	// defaultTemplateUpdateRequestMaxPoolSize is default max poolsize for varTemplateUpdateRequestMaxPoolSize
	defaultTemplateUpdateRequestMaxPoolSize = "5"

	// ToolchainConfigMapUserApprovalPolicy is a key for a user approval policy that should be used
	ToolchainConfigMapUserApprovalPolicy = "user-approval-policy"

	// ToolchainConfigMapName specifies a name of a ConfigMap that keeps toolchain configuration
	ToolchainConfigMapName = "toolchain-saas-config"

	// UserApprovalPolicyManual defines that the new users should be approved manually
	UserApprovalPolicyManual = "manual"

	// UserApprovalPolicyAutomatic defines that the new users should be approved automatically
	UserApprovalPolicyAutomatic = "automatic"

	// NotificationDeliveryServiceMailgun is the notification delivery service to use during production
	NotificationDeliveryServiceMailgun = "mailgun"

	// varNotificationDeliveryService specifies the duration before a notification is deleted
	varNotificationDeliveryService = "notification.delivery.service"

	// varDurationBeforeNotificationDeletion specifies the duration before a notification will be deleted
	varDurationBeforeNotificationDeletion = "duration.before.notification.deletion"

	// defaultDurationBeforeNotificationDeletion is the time before a Notification resource is deleted
	defaultDurationBeforeNotificationDeletion = "24h"

	// varMailgunDomain specifies the host operator mailgun domain used for creating an instance of mailgun
	varMailgunDomain = "mailgun.domain"

	// varMailgunAPIKey specifies the host operator mailgun api key used for creating an instance of mailgun
	varMailgunAPIKey = "mailgun.api.key"

	// varMailgunSenderEmail specifies the host operator mailgun senders email
	varMailgunSenderEmail = "mailgun.sender.email"

	// varRegistrationServiceURL is the URL used to access the registration service
	varRegistrationServiceURL = "registration.service.url"

	// defaultRegistrationServiceURL is the default location of the registration service
	defaultRegistrationServiceURL = "https://registration.crt-placeholder.com"

	// varEnvironment specifies the host-operator environment such as prod, stage, unit-tests, e2e-tests, dev, etc
	varEnvironment = "environment"

	// defaultEnvironment is the default host-operator environment
	defaultEnvironment = "prod"

	// varMasterUserRecordUpdateFailureThreshold specifies the number allowed failures before stopping trying to update a MasterUserRecord
	varMasterUserRecordUpdateFailureThreshold = "masteruserrecord.update.failure.threshold"
)

// Config encapsulates the Viper configuration registry which stores the
// configuration data in-memory.
type Config struct {
	host         *viper.Viper
	secretValues map[string]string
}

var log = logf.Log.WithName("configuration")

// initConfig creates an initial, empty configuration.
func initConfig(secret map[string]string) *Config {
	c := Config{
		host:         viper.New(),
		secretValues: secret,
	}
	c.host.SetEnvPrefix(HostEnvPrefix)
	c.host.AutomaticEnv()
	c.host.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	c.host.SetTypeByDefaultValue(true)
	c.setConfigDefaults()

	return &c
}

func LoadConfig(cl client.Client) (*Config, error) {

	secret, err := configuration.LoadFromSecret(HostEnvPrefix, "HOST_OPERATOR_SECRET_NAME", cl)
	if err != nil {
		return nil, err
	}

	err = configuration.LoadFromConfigMap(HostEnvPrefix, "HOST_OPERATOR_CONFIG_MAP_NAME", cl)
	if err != nil {
		return nil, err
	}

	return initConfig(secret), nil
}

func (c *Config) setConfigDefaults() {
	c.host.SetTypeByDefaultValue(true)
	c.host.SetDefault(ToolchainStatusName, DefaultToolchainStatusName)
	c.host.SetDefault(VarDurationBeforeChangeRequestDeletion, defaultDurationBeforeChangeTierRequestDeletion)
	c.host.SetDefault(varTemplateUpdateRequestMaxPoolSize, defaultTemplateUpdateRequestMaxPoolSize)
	c.host.SetDefault(varNotificationDeliveryService, NotificationDeliveryServiceMailgun)
	c.host.SetDefault(varDurationBeforeNotificationDeletion, defaultDurationBeforeNotificationDeletion)
	c.host.SetDefault(varRegistrationServiceURL, defaultRegistrationServiceURL)
	c.host.SetDefault(varEnvironment, defaultEnvironment)
	c.host.SetDefault(varMasterUserRecordUpdateFailureThreshold, 2) // allow 1 failure, try again and then give up if failed again
}

// GetToolchainStatusName returns the configured name of the member status resource
func (c *Config) GetToolchainStatusName() string {
	return c.host.GetString(ToolchainStatusName)
}

// GetDurationBeforeChangeTierRequestDeletion returns the timeout before a complete TierChangeRequest will be deleted.
func (c *Config) GetDurationBeforeChangeTierRequestDeletion() time.Duration {
	return c.host.GetDuration(VarDurationBeforeChangeRequestDeletion)
}

// GetNotificationDeliveryService returns the name of the notification delivery service to use for delivering user notifications
func (c *Config) GetNotificationDeliveryService() string {
	return c.host.GetString(varNotificationDeliveryService)
}

// GetTemplateUpdateRequestMaxPoolSize returns the maximum number of concurrent TemplateUpdateRequests when updating MasterUserRecords
func (c *Config) GetTemplateUpdateRequestMaxPoolSize() int {
	return c.host.GetInt(varTemplateUpdateRequestMaxPoolSize)
}

// GetDurationBeforeNotificationDeletion returns the timeout before a delivered notification will be deleted.
func (c *Config) GetDurationBeforeNotificationDeletion() time.Duration {
	return c.host.GetDuration(varDurationBeforeNotificationDeletion)
}

// GetMailgunDomain returns the host operator mailgun domain
func (c *Config) GetMailgunDomain() string {
	key := configuration.CreateOperatorEnvVarKey(HostEnvPrefix, varMailgunDomain)
	return c.secretValues[key]
}

// GetMailgunAPIKey returns the host operator mailgun api key
func (c *Config) GetMailgunAPIKey() string {
	key := configuration.CreateOperatorEnvVarKey(HostEnvPrefix, varMailgunAPIKey)
	return c.secretValues[key]
}

// GetMailgunSenderEmail returns the host operator mailgun sender's email address
func (c *Config) GetMailgunSenderEmail() string {
	key := configuration.CreateOperatorEnvVarKey(HostEnvPrefix, varMailgunSenderEmail)
	return c.secretValues[key]
}

// GetRegistrationServiceURL returns the URL of the registration service
func (c *Config) GetRegistrationServiceURL() string {
	return c.host.GetString(varRegistrationServiceURL)
}

// GetEnvironment returns the host-operator environment such as prod, stage, unit-tests, e2e-tests, dev, etc
func (c *Config) GetEnvironment() string {
	return c.host.GetString(varEnvironment)
}

// GetMasterUserRecordUpdateFailureThreshold returns the number of allowed failures before stopping trying to update a MasterUserRecord
func (c *Config) GetMasterUserRecordUpdateFailureThreshold() int {
	return c.host.GetInt(varMasterUserRecordUpdateFailureThreshold)
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
