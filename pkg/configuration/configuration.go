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

var log = logf.Log.WithName("configuration")

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

	// varDurationBeforeChangeRequestDeletion specifies the duration before a ChangeTierRequest resource is deleted
	varDurationBeforeChangeRequestDeletion = "duration.before.change.request.deletion" // TODO: add missing `tier` in `change.tier.request`?

	// defaultDurationBeforeChangeTierRequestDeletion is the time before a ChangeTierRequest resource is deleted
	defaultDurationBeforeChangeTierRequestDeletion = "24h"

	// varRegistrationServiceURL is the URL used to access the registration service
	varRegistrationServiceURL = "registration.service.url"

	// defaultRegistrationServiceURL is the default location of the registration service
	defaultRegistrationServiceURL = "https://registration.crt-placeholder.com"

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

	// varConsoleNamespace is the console route namespace
	varConsoleNamespace = "console.namespace"

	// defaultConsoleNamespace is the default console route namespace
	defaultConsoleNamespace = "openshift-console"

	// varConsoleRouteName is the console route name
	varConsoleRouteName = "console.route.name"

	// defaultConsoleRouteName is the default console route name
	defaultConsoleRouteName = "console"

	// varCheNamespace is the che route namespace
	varCheNamespace = "che.namespace"

	// defaultCheNamespace is the default che route namespace
	defaultCheNamespace = "toolchain-che"

	// varCheRouteName is the che dashboard route
	varCheRouteName = "che.route.name"

	// defaultCheRouteName is the default che dashboard route
	defaultCheRouteName = "che"

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

	secret, err := configuration.LoadFromSecret("HOST_OPERATOR_SECRET_NAME", cl)
	if err != nil {
		return nil, err
	}

	err = configuration.LoadFromConfigMap(HostEnvPrefix, "HOST_OPERATOR_CONFIG_MAP_NAME", cl)
	if err != nil {
		return nil, err
	}

	return initConfig(secret), nil
}

func getHostEnvVarKey(key string) string {
	envKey := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	return HostEnvPrefix + "_" + envKey
}

func (c *Config) PrintConfig() {
	logWithValuesRegServ := log
	for key, value := range c.GetAllRegistrationServiceParameters() {
		logWithValuesRegServ = logWithValuesRegServ.WithValues(key, value)
	}

	logWithValuesRegServ.Info("Please note that these variables can be overridden by the registration-service and " +
		"hence may differ from the actual values used in the registration-service. Registration Service configuration variables:")

	logWithValuesHost := log.WithValues(
		getHostEnvVarKey(varDurationBeforeChangeRequestDeletion), c.GetDurationBeforeChangeTierRequestDeletion(),
		getHostEnvVarKey(varRegistrationServiceURL), c.GetRegistrationServiceURL())

	logWithValuesHost.Info("Host Operator configuration variables:")
}

func (c *Config) setConfigDefaults() {
	c.host.SetTypeByDefaultValue(true)
	c.host.SetDefault(ToolchainStatusName, DefaultToolchainStatusName)
	c.host.SetDefault(varDurationBeforeChangeRequestDeletion, defaultDurationBeforeChangeTierRequestDeletion)
	c.host.SetDefault(varRegistrationServiceURL, defaultRegistrationServiceURL)
	c.host.SetDefault(varTemplateUpdateRequestMaxPoolSize, defaultTemplateUpdateRequestMaxPoolSize)
	c.host.SetDefault(varNotificationDeliveryService, NotificationDeliveryServiceMailgun)
	c.host.SetDefault(varDurationBeforeNotificationDeletion, defaultDurationBeforeNotificationDeletion)
	c.host.SetDefault(varConsoleNamespace, defaultConsoleNamespace)
	c.host.SetDefault(varConsoleRouteName, defaultConsoleRouteName)
	c.host.SetDefault(varCheNamespace, defaultCheNamespace)
	c.host.SetDefault(varCheRouteName, defaultCheRouteName)
	c.host.SetDefault(varEnvironment, defaultEnvironment)
	c.host.SetDefault(varMasterUserRecordUpdateFailureThreshold, 2) // allow 1 failure, try again and then give up if failed again
}

// GetToolchainStatusName returns the configured name of the member status resource
func (c *Config) GetToolchainStatusName() string {
	return c.host.GetString(ToolchainStatusName)
}

// GetDurationBeforeChangeTierRequestDeletion returns the timeout before a complete TierChangeRequest will be deleted.
func (c *Config) GetDurationBeforeChangeTierRequestDeletion() time.Duration {
	return c.host.GetDuration(varDurationBeforeChangeRequestDeletion)
}

// GetRegistrationServiceURL returns the URL of the registration service
func (c *Config) GetRegistrationServiceURL() string {
	return c.host.GetString(varRegistrationServiceURL)
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
	return c.secretValues[varMailgunDomain]
}

// GetMailgunAPIKey returns the host operator mailgun api key
func (c *Config) GetMailgunAPIKey() string {
	return c.secretValues[varMailgunAPIKey]
}

// GetMailgunSenderEmail returns the host operator mailgun sender's email address
func (c *Config) GetMailgunSenderEmail() string {
	return c.secretValues[varMailgunSenderEmail]
}

// GetConsoleNamespace returns the console route namespace
func (c *Config) GetConsoleNamespace() string {
	return c.host.GetString(varConsoleNamespace)
}

// GetConsoleRouteName returns the console route name
func (c *Config) GetConsoleRouteName() string {
	return c.host.GetString(varConsoleRouteName)
}

// GetCheNamespace returns the Che route namespace
func (c *Config) GetCheNamespace() string {
	return c.host.GetString(varCheNamespace)
}

// GetCheRouteName returns the name of the Che dashboard route
func (c *Config) GetCheRouteName() string {
	return c.host.GetString(varCheRouteName)
}

// GetEnvironment returns the host-operator environment such as prod, stage, unit-tests, e2e-tests, dev, etc
func (c *Config) GetEnvironment() string {
	return c.host.GetString(varEnvironment)
}

// GetMasterUserRecordUpdateFailureThreshold returns the number of allowed failures before stopping trying to update a MasterUserRecord
func (c *Config) GetMasterUserRecordUpdateFailureThreshold() int {
	return c.host.GetInt(varMasterUserRecordUpdateFailureThreshold)
}

// GetAllRegistrationServiceParameters returns a map with key-values pairs of parameters that have REGISTRATION_SERVICE prefix
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
