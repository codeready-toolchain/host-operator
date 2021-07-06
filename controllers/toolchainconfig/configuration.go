package toolchainconfig

import (
	"os"
	"strings"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// RegServiceEnvPrefix will be used for registration service environment variable name prefixing.
	RegServiceEnvPrefix = "REGISTRATION_SERVICE"

	ToolchainStatusName = "toolchain-status"

	// NotificationDeliveryServiceMailgun is the notification delivery service to use during production
	NotificationDeliveryServiceMailgun = "mailgun"
)

var log = logf.Log.WithName("configuration")

type ToolchainConfig struct {
	cfg     *toolchainv1alpha1.ToolchainConfigSpec
	secrets map[string]map[string]string
}

func NewToolchainConfig(cfg *toolchainv1alpha1.ToolchainConfigSpec) ToolchainConfig {
	return ToolchainConfig{
		cfg: cfg,
	}
}

func (c *ToolchainConfig) Print() {
	log.Info("Toolchain configuration variables", "ToolchainConfigSpec", c.cfg)
}

func (c *ToolchainConfig) Environment() string {
	return getString(c.cfg.Host.Environment, "prod")
}

func (c *ToolchainConfig) AutomaticApproval() AutoApprovalConfig {
	return AutoApprovalConfig{c.cfg.Host.AutomaticApproval}
}

func (c *ToolchainConfig) Deactivation() DeactivationConfig {
	return DeactivationConfig{c.cfg.Host.Deactivation}
}

func (c *ToolchainConfig) Metrics() MetricsConfig {
	return MetricsConfig{c.cfg.Host.Metrics}
}

func (c *ToolchainConfig) Notifications() NotificationsConfig {
	return NotificationsConfig{
		c:       c.cfg.Host.Notifications,
		secrets: c.secrets,
	}
}

func (c *ToolchainConfig) RegistrationService() RegistrationServiceConfig {
	return RegistrationServiceConfig{c.cfg.Host.RegistrationService}
}

func (c *ToolchainConfig) Tiers() TiersConfig {
	return TiersConfig{c.cfg.Host.Tiers}
}

func (c *ToolchainConfig) ToolchainStatus() ToolchainStatusConfig {
	return ToolchainStatusConfig{c.cfg.Host.ToolchainStatus}
}

func (c *ToolchainConfig) Users() UsersConfig {
	return UsersConfig{c.cfg.Host.Users}
}

// GetAllRegistrationServiceParameters returns a map with key-values pairs of parameters that have REGISTRATION_SERVICE prefix
func GetAllRegistrationServiceParameters() map[string]string {
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

type AutoApprovalConfig struct {
	approval toolchainv1alpha1.AutomaticApprovalConfig
}

func (a AutoApprovalConfig) IsEnabled() bool {
	return getBool(a.approval.Enabled, false)
}

func (a AutoApprovalConfig) ResourceCapacityThresholdDefault() int {
	return getInt(a.approval.ResourceCapacityThreshold.DefaultThreshold, 80)
}

func (a AutoApprovalConfig) ResourceCapacityThresholdSpecificPerMemberCluster() map[string]int {
	return a.approval.ResourceCapacityThreshold.SpecificPerMemberCluster
}

func (a AutoApprovalConfig) MaxNumberOfUsersOverall() int {
	return getInt(a.approval.MaxNumberOfUsers.Overall, 1000)
}

func (a AutoApprovalConfig) MaxNumberOfUsersSpecificPerMemberCluster() map[string]int {
	return a.approval.MaxNumberOfUsers.SpecificPerMemberCluster
}

type DeactivationConfig struct {
	dctv toolchainv1alpha1.DeactivationConfig
}

func (d DeactivationConfig) DeactivatingNotificationDays() int {
	return getInt(d.dctv.DeactivatingNotificationDays, 3)
}

func (d DeactivationConfig) DeactivationDomainsExcluded() []string {
	excluded := getString(d.dctv.DeactivationDomainsExcluded, "")
	v := strings.FieldsFunc(excluded, func(c rune) bool {
		return c == ','
	})
	return v
}

func (d DeactivationConfig) UserSignupDeactivatedRetentionDays() int {
	return getInt(d.dctv.UserSignupDeactivatedRetentionDays, 365)
}

func (d DeactivationConfig) UserSignupUnverifiedRetentionDays() int {
	return getInt(d.dctv.UserSignupUnverifiedRetentionDays, 7)
}

type MetricsConfig struct {
	metrics toolchainv1alpha1.MetricsConfig
}

func (d MetricsConfig) ForceSynchronization() bool {
	return getBool(d.metrics.ForceSynchronization, false)
}

type NotificationsConfig struct {
	c       toolchainv1alpha1.NotificationsConfig
	secrets map[string]map[string]string
}

func (n NotificationsConfig) notificationSecret(secretKey string) string {
	secret := getString(n.c.Secret.Ref, "")
	return n.secrets[secret][secretKey]
}

func (n NotificationsConfig) NotificationDeliveryService() string {
	return getString(n.c.NotificationDeliveryService, "mailgun")
}

func (n NotificationsConfig) DurationBeforeNotificationDeletion() time.Duration {
	v := getString(n.c.DurationBeforeNotificationDeletion, "24h")
	duration, err := time.ParseDuration(v)
	if err != nil {
		duration = 24 * time.Hour
	}
	return duration
}

func (n NotificationsConfig) AdminEmail() string {
	return getString(n.c.AdminEmail, "")
}

func (n NotificationsConfig) MailgunDomain() string {
	key := getString(n.c.Secret.MailgunDomain, "")
	return n.notificationSecret(key)
}

func (n NotificationsConfig) MailgunAPIKey() string {
	key := getString(n.c.Secret.MailgunAPIKey, "")
	return n.notificationSecret(key)
}

func (n NotificationsConfig) MailgunSenderEmail() string {
	key := getString(n.c.Secret.MailgunSenderEmail, "")
	return n.notificationSecret(key)
}

func (n NotificationsConfig) MailgunReplyToEmail() string {
	key := getString(n.c.Secret.MailgunReplyToEmail, "")
	return n.notificationSecret(key)
}

type RegistrationServiceConfig struct {
	c toolchainv1alpha1.RegistrationServiceConfig
}

func (d RegistrationServiceConfig) RegistrationServiceURL() string {
	return getString(d.c.RegistrationServiceURL, "https://registration.crt-placeholder.com")
}

type TiersConfig struct {
	tiers toolchainv1alpha1.TiersConfig
}

func (d TiersConfig) DurationBeforeChangeTierRequestDeletion() time.Duration {
	v := getString(d.tiers.DurationBeforeChangeTierRequestDeletion, "24h")
	duration, err := time.ParseDuration(v)
	if err != nil {
		duration = 24 * time.Hour
	}
	return duration
}

func (d TiersConfig) TemplateUpdateRequestMaxPoolSize() int {
	return getInt(d.tiers.TemplateUpdateRequestMaxPoolSize, 5)
}

type ToolchainStatusConfig struct {
	t toolchainv1alpha1.ToolchainStatusConfig
}

func (d ToolchainStatusConfig) ToolchainStatusRefreshTime() time.Duration {
	v := getString(d.t.ToolchainStatusRefreshTime, "5s")
	duration, err := time.ParseDuration(v)
	if err != nil {
		duration = 5 * time.Second
	}
	return duration
}

type UsersConfig struct {
	c toolchainv1alpha1.UsersConfig
}

func (d UsersConfig) MasterUserRecordUpdateFailureThreshold() int {
	return getInt(d.c.MasterUserRecordUpdateFailureThreshold, 2) // default: allow 1 failure, try again and then give up if failed again
}

func (d UsersConfig) ForbiddenUsernamePrefixes() []string {
	prefixes := getString(d.c.ForbiddenUsernamePrefixes, "openshift,kube,default,redhat,sandbox")
	v := strings.FieldsFunc(prefixes, func(c rune) bool {
		return c == ','
	})
	return v
}

func (d UsersConfig) ForbiddenUsernameSuffixes() []string {
	suffixes := getString(d.c.ForbiddenUsernameSuffixes, "admin")
	v := strings.FieldsFunc(suffixes, func(c rune) bool {
		return c == ','
	})
	return v
}

func getBool(value *bool, defaultValue bool) bool {
	if value != nil {
		return *value
	}
	return defaultValue
}

func getInt(value *int, defaultValue int) int {
	if value != nil {
		return *value
	}
	return defaultValue
}

func getString(value *string, defaultValue string) string {
	if value != nil {
		return *value
	}
	return defaultValue
}
