// Package configuration is in charge of the validation and extraction of all
// the configuration details from a configuration file or environment variables.
package configuration

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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

	// VarDurationBeforeChangeRequestDeletion specifies the duration before a change request is deleted
	VarDurationBeforeChangeRequestDeletion = "duration.before.change.request.deletion"

	// defaultDurationBeforeDeletion is the time before a resource is deleted
	defaultDurationBeforeDeletion = "24h"

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

	// varMasterUserRecordUpdateRetries specifies the number of retries before failing to update a MasterUserRecord
	varMasterUserRecordUpdateRetries = "masteruserrecord.update.retries"
)

// Config encapsulates the Viper configuration registry which stores the
// configuration data in-memory.
type Config struct {
	host *viper.Viper
}

var log = logf.Log.WithName("configuration")

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

func LoadConfig(cl client.Client) (*Config, error) {
	err := loadFromSecret(cl)
	if err != nil {
		return nil, err
	}

	err = loadFromConfigMap(cl)
	if err != nil {
		return nil, err
	}
	return initConfig(), nil
}

// loadFromSecret retrieves the host operator secret
func loadFromSecret(cl client.Client) error {
	// get the secret name
	secretName := getResourceName("HOST_OPERATOR_SECRET_NAME")
	if secretName == "" {
		return nil
	}

	// get namespace
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		return err
	}

	// get the secret
	secret := &v1.Secret{}
	namespacedName := types.NamespacedName{Namespace: namespace, Name: secretName}
	err = client.Client.Get(cl, context.TODO(), namespacedName, secret)
	if err != nil {
		return err
	}

	// get secrets and set environment variables
	for key, value := range secret.Data {
		secretKey := createHostEnvVarKey(key)
		err := os.Setenv(secretKey, string(value))
		if err != nil {
			return err
		}
	}

	return nil
}

// loadFromConfigMap retrieves the host operator configMap
func loadFromConfigMap(cl client.Client) error {
	// get the configMap name
	configMapName := getResourceName("HOST_OPERATOR_CONFIG_MAP_NAME")
	if configMapName == "" {
		return nil
	}
	// get namespace
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		return err
	}

	// get the configMap
	configMap := &v1.ConfigMap{}
	namespacedName := types.NamespacedName{Namespace: namespace, Name: configMapName}
	err = client.Client.Get(cl, context.TODO(), namespacedName, configMap)
	if err != nil {
		return err
	}

	// get configMap data and set environment variables
	for key, value := range configMap.Data {
		configKey := createHostEnvVarKey(key)
		err := os.Setenv(configKey, value)
		if err != nil {
			return err
		}
	}

	return nil
}

// getResourceName gets the resource name via env var
func getResourceName(key string) string {
	// get the resource name
	resourceName := os.Getenv(key)
	if resourceName == "" {
		log.Info(key + " is not set")
		return ""
	}

	return resourceName
}

// createHostEnvVarKey creates env vars based on resource data
func createHostEnvVarKey(key string) string {
	return HostEnvPrefix + "_" + (strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(key, ".", "_"), "-", "_")))
}

func (c *Config) setConfigDefaults() {
	c.host.SetTypeByDefaultValue(true)
	c.host.SetDefault(ToolchainStatusName, DefaultToolchainStatusName)
	c.host.SetDefault(VarDurationBeforeChangeRequestDeletion, defaultDurationBeforeDeletion)
	c.host.SetDefault(varNotificationDeliveryService, NotificationDeliveryServiceMailgun)
	c.host.SetDefault(varDurationBeforeNotificationDeletion, defaultDurationBeforeDeletion)
	c.host.SetDefault(varRegistrationServiceURL, defaultRegistrationServiceURL)
	c.host.SetDefault(varEnvironment, defaultEnvironment)
	c.host.SetDefault(varMasterUserRecordUpdateRetries, 1)
}

// GetToolchainStatusName returns the configured name of the member status resource
func (c *Config) GetToolchainStatusName() string {
	return c.host.GetString(ToolchainStatusName)
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

// GetMailgunDomain returns the host operator mailgun domain
func (c *Config) GetMailgunDomain() string {
	return c.host.GetString(varMailgunDomain)
}

// GetMailgunAPIKey returns the host operator mailgun api key
func (c *Config) GetMailgunAPIKey() string {
	return c.host.GetString(varMailgunAPIKey)
}

// GetMailgunSenderEmail returns the host operator mailgun sender's email address
func (c *Config) GetMailgunSenderEmail() string {
	return c.host.GetString(varMailgunSenderEmail)
}

// GetRegistrationServiceURL returns the URL of the registration service
func (c *Config) GetRegistrationServiceURL() string {
	return c.host.GetString(varRegistrationServiceURL)
}

// GetEnvironment returns the host-operator environment such as prod, stage, unit-tests, e2e-tests, dev, etc
func (c *Config) GetEnvironment() string {
	return c.host.GetString(varEnvironment)
}

// GetMasterUserRecordUpdateRetries returns the number of retries before failing to update a MasterUserRecord
func (c *Config) GetMasterUserRecordUpdateRetries() int {
	return c.host.GetInt(varMasterUserRecordUpdateRetries)
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
