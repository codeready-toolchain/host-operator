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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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

	// NotificationDeliveryServiceMailgun is the notification delivery service to use during production
	NotificationDeliveryServiceMailgun = "mailgun"

	// varNotificationDeliveryService specifies the duration before a notification is deleted
	varNotificationDeliveryService = "notification.delivery.service"

	// varDurationBeforeNotificationDeletion specifies the duration before a notification will be deleted
	varDurationBeforeNotificationDeletion = "duration.before.notification.deletion"

	// defaultDurationBeforeNotificationDeletion is the time before a Notification resource is deleted
	defaultDurationBeforeNotificationDeletion = "24h"

	// varAdminEmail specifies the administrator email address for system notifications
	varAdminEmail = "admin.email"

	// varMailgunDomain specifies the host operator mailgun domain used for creating an instance of mailgun
	varMailgunDomain = "mailgun.domain"

	// varMailgunAPIKey specifies the host operator mailgun api key used for creating an instance of mailgun
	varMailgunAPIKey = "mailgun.api.key"

	// varMailgunSenderEmail specifies the host operator mailgun senders email
	varMailgunSenderEmail = "mailgun.sender.email"

	// varMailgunReplyToEmail specifies the reply-to email address that will be set in sent notifications
	varMailgunReplyToEmail = "mailgun.replyto.email"

	// varEnvironment specifies the host-operator environment such as prod, stage, unit-tests, e2e-tests, dev, etc
	varEnvironment = "environment"

	// defaultEnvironment is the default host-operator environment
	defaultEnvironment = "prod"

	// varMasterUserRecordUpdateFailureThreshold specifies the number allowed failures before stopping trying to update a MasterUserRecord
	varMasterUserRecordUpdateFailureThreshold = "masteruserrecord.update.failure.threshold"

	// varToolchainStatusRefreshTime specifies how often the ToolchainStatus should load and refresh the current hosted-toolchain status
	varToolchainStatusRefreshTime = "toolchainstatus.refresh.time"

	// defaultToolchainStatusRefreshTime is the default refresh period for ToolchainStatus
	defaultToolchainStatusRefreshTime = "5s"

	// varDeactivationDomainsExcluded is a string of comma-separated domains that should be excluded from automatic user deactivation
	// For example: "@redhat.com,@ibm.com"
	varDeactivationDomainsExcluded = "deactivation.domains.excluded"

	// varForbiddenUsernamePrefixes defines the prefixes that a username may not have when signing up.  If a
	// username has a forbidden prefix, then the username compliance prefix is added to the username
	varForbiddenUsernamePrefixes = "username.forbidden.prefixes"

	DefaultForbiddenUsernamePrefixes = "openshift,kube,default,redhat,sandbox"

	// varForbiddenUsernameSuffixes defines the suffixes that a username may not have when signing up.  If a
	// username has a forbidden suffix, then the username compliance suffix is added to the username
	varForbiddenUsernameSuffixes = "username.forbidden.suffixes"

	DefaultForbiddenUsernameSuffixes = "admin"

	// varUserSignupUnverifiedRetentionDays is used to configure how many days we should keep unverified (i.e. the user
	// hasn't completed the user verification process via the registration service) UserSignup resources before deleting
	// them.  It is intended for this parameter to define an aggressive cleanup schedule for unverified user signups,
	// and the default configuration value for this parameter reflects this.
	varUserSignupUnverifiedRetentionDays = "usersignup.unverified.retention.days"

	defaultUserSignupUnverifiedRetentionDays = 7

	// varUserSignupDeactivatedRetentionDays is used to configure how many days we should keep deactivated UserSignup
	// resources before deleting them.  This parameter value should reflect an extended period of time sufficient for
	// gathering user metrics before removing the resources from the cluster.
	varUserSignupDeactivatedRetentionDays = "usersignup.deactivated.retention.days"

	defaultUserSignupDeactivatedRetentionDays = 180
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
	c.host.SetDefault(varEnvironment, defaultEnvironment)
	c.host.SetDefault(varMasterUserRecordUpdateFailureThreshold, 2) // allow 1 failure, try again and then give up if failed again
	c.host.SetDefault(varToolchainStatusRefreshTime, defaultToolchainStatusRefreshTime)
	c.host.SetDefault(varForbiddenUsernamePrefixes, strings.FieldsFunc(DefaultForbiddenUsernamePrefixes, func(c rune) bool {
		return c == ','
	}))
	c.host.SetDefault(varForbiddenUsernameSuffixes, strings.FieldsFunc(DefaultForbiddenUsernameSuffixes, func(c rune) bool {
		return c == ','
	}))
	c.host.SetDefault(varUserSignupUnverifiedRetentionDays, defaultUserSignupUnverifiedRetentionDays)
	c.host.SetDefault(varUserSignupDeactivatedRetentionDays, defaultUserSignupDeactivatedRetentionDays)
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

// GetAdminEmail returns the email address for administrative notifications
func (c *Config) GetAdminEmail() string {
	return c.host.GetString(varAdminEmail)
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

// GetMailgunReplyToEmail returns the (optional) reply-to email address to set in sent notifications
func (c *Config) GetMailgunReplyToEmail() string {
	return c.secretValues[varMailgunReplyToEmail]
}

// GetEnvironment returns the host-operator environment such as prod, stage, unit-tests, e2e-tests, dev, etc
func (c *Config) GetEnvironment() string {
	return c.host.GetString(varEnvironment)
}

// GetMasterUserRecordUpdateFailureThreshold returns the number of allowed failures before stopping trying to update a MasterUserRecord
func (c *Config) GetMasterUserRecordUpdateFailureThreshold() int {
	return c.host.GetInt(varMasterUserRecordUpdateFailureThreshold)
}

// GetToolchainStatusRefreshTime returns the time how often the ToolchainStatus should load and refresh the current hosted-toolchain status
func (c *Config) GetToolchainStatusRefreshTime() time.Duration {
	return c.host.GetDuration(varToolchainStatusRefreshTime)
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

// GetDeactivationDomainsExcludedList returns a string of comma-separated user email domains that should be excluded from automatic user deactivation
func (c *Config) GetDeactivationDomainsExcludedList() []string {
	return strings.FieldsFunc(c.host.GetString(varDeactivationDomainsExcluded), func(c rune) bool {
		return c == ','
	})
}

func (c *Config) GetForbiddenUsernamePrefixes() []string {
	return c.host.GetStringSlice(varForbiddenUsernamePrefixes)
}

func (c *Config) GetForbiddenUsernameSuffixes() []string {
	return c.host.GetStringSlice(varForbiddenUsernameSuffixes)
}

func (c *Config) GetUserSignupUnverifiedRetentionDays() int {
	return c.host.GetInt(varUserSignupUnverifiedRetentionDays)
}

func (c *Config) GetUserSignupDeactivatedRetentionDays() int {
	return c.host.GetInt(varUserSignupDeactivatedRetentionDays)
}
