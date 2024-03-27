package toolchainconfig

import (
	"fmt"
	"strings"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"

	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ToolchainStatusName = "toolchain-status"

	// NotificationDeliveryServiceMailgun is the notification delivery service to use during production
	NotificationDeliveryServiceMailgun = "mailgun"

	NotificationContextRegistrationURLKey = "RegistrationURL"
)

var logger = logf.Log.WithName("toolchainconfig")

type ToolchainConfig struct {
	cfg     *toolchainv1alpha1.ToolchainConfigSpec
	secrets map[string]map[string]string
}

// GetToolchainConfig returns a ToolchainConfig using the cache, or if the cache was not initialized
// then retrieves the latest config using the provided client and updates the cache
func GetToolchainConfig(cl runtimeclient.Client) (ToolchainConfig, error) {
	config, secrets, err := commonconfig.GetConfig(cl, &toolchainv1alpha1.ToolchainConfig{})
	if err != nil {
		// return default config
		logger.Error(err, "failed to retrieve ToolchainConfig")
		return ToolchainConfig{cfg: &toolchainv1alpha1.ToolchainConfigSpec{}}, err
	}
	return newToolchainConfig(config, secrets), nil
}

// GetCachedToolchainConfig returns a ToolchainConfig directly from the cache
func GetCachedToolchainConfig() ToolchainConfig {
	config, secrets := commonconfig.GetCachedConfig()
	return newToolchainConfig(config, secrets)
}

// ForceLoadToolchainConfig updates the cache using the provided client and returns the latest ToolchainConfig
func ForceLoadToolchainConfig(cl runtimeclient.Client) (ToolchainConfig, error) {
	config, secrets, err := commonconfig.LoadLatest(cl, &toolchainv1alpha1.ToolchainConfig{})
	if err != nil {
		// return default config
		logger.Error(err, "failed to force load ToolchainConfig")
		return ToolchainConfig{cfg: &toolchainv1alpha1.ToolchainConfigSpec{}}, err
	}
	return newToolchainConfig(config, secrets), nil
}

func newToolchainConfig(config runtime.Object, secrets map[string]map[string]string) ToolchainConfig {
	if config == nil {
		// return default config if there's no config resource
		return ToolchainConfig{cfg: &toolchainv1alpha1.ToolchainConfigSpec{}}
	}

	toolchaincfg, ok := config.(*toolchainv1alpha1.ToolchainConfig)
	if !ok {
		// return default config
		logger.Error(fmt.Errorf("cache does not contain toolchainconfig resource type"), "failed to get ToolchainConfig from resource, using default configuration")
		return ToolchainConfig{cfg: &toolchainv1alpha1.ToolchainConfigSpec{}}
	}
	return ToolchainConfig{cfg: &toolchaincfg.Spec, secrets: secrets}
}

func (c *ToolchainConfig) Print() {
	logger.Info("Toolchain configuration", "config", c.cfg)
}

func (c *ToolchainConfig) Environment() string {
	return commonconfig.GetString(c.cfg.Host.Environment, "prod")
}

func (c *ToolchainConfig) GitHubSecret() GitHubSecret {
	return GitHubSecret{
		s:       c.cfg.Host.ToolchainStatus.GitHubSecret,
		secrets: c.secrets,
	}
}

func (c *ToolchainConfig) AutomaticApproval() AutoApprovalConfig {
	return AutoApprovalConfig{c.cfg.Host.AutomaticApproval}
}

// Deprecated: This is no longer used for anything.
func (c *ToolchainConfig) CapacityThresholds() CapacityThresholdsConfig {
	return CapacityThresholdsConfig{c.cfg.Host.CapacityThresholds}
}

func (c *ToolchainConfig) SpaceConfig() SpaceConfig {
	return SpaceConfig{c.cfg.Host.SpaceConfig}
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

type AutoApprovalConfig struct {
	approval toolchainv1alpha1.AutomaticApprovalConfig
}

func (a AutoApprovalConfig) IsEnabled() bool {
	return commonconfig.GetBool(a.approval.Enabled, false)
}

type SpaceConfig struct {
	spaceConfig toolchainv1alpha1.SpaceConfig
}

func (s SpaceConfig) SpaceRequestIsEnabled() bool {
	return commonconfig.GetBool(s.spaceConfig.SpaceRequestEnabled, false)
}

func (s SpaceConfig) SpaceBindingRequestIsEnabled() bool {
	return commonconfig.GetBool(s.spaceConfig.SpaceBindingRequestEnabled, false)
}

// Deprecated: This is no longer used for anything.
type CapacityThresholdsConfig struct {
	capacityThresholds toolchainv1alpha1.CapacityThresholds
}

// Deprecated: This is no longer used for anything.
func (c CapacityThresholdsConfig) MaxNumberOfSpacesSpecificPerMemberCluster() map[string]int {
	return c.capacityThresholds.MaxNumberOfSpacesPerMemberCluster
}

// Deprecated: This is no longer used for anything.
func (c CapacityThresholdsConfig) ResourceCapacityThresholdDefault() int {
	return commonconfig.GetInt(c.capacityThresholds.ResourceCapacityThreshold.DefaultThreshold, 80)
}

// Deprecated: This is no longer used for anything.
func (c CapacityThresholdsConfig) ResourceCapacityThresholdSpecificPerMemberCluster() map[string]int {
	return c.capacityThresholds.ResourceCapacityThreshold.SpecificPerMemberCluster
}

type DeactivationConfig struct {
	dctv toolchainv1alpha1.DeactivationConfig
}

func (d DeactivationConfig) DeactivatingNotificationDays() int {
	return commonconfig.GetInt(d.dctv.DeactivatingNotificationDays, 3)
}

func (d DeactivationConfig) DeactivationDomainsExcluded() []string {
	excluded := commonconfig.GetString(d.dctv.DeactivationDomainsExcluded, "")
	v := strings.FieldsFunc(excluded, func(c rune) bool {
		return c == ','
	})
	return v
}

func (d DeactivationConfig) UserSignupDeactivatedRetentionDays() int {
	return commonconfig.GetInt(d.dctv.UserSignupDeactivatedRetentionDays, 1460)
}

func (d DeactivationConfig) UserSignupUnverifiedRetentionDays() int {
	return commonconfig.GetInt(d.dctv.UserSignupUnverifiedRetentionDays, 7)
}

type MetricsConfig struct {
	metrics toolchainv1alpha1.MetricsConfig
}

func (d MetricsConfig) ForceSynchronization() bool {
	return commonconfig.GetBool(d.metrics.ForceSynchronization, false)
}

type GitHubSecret struct {
	s       toolchainv1alpha1.GitHubSecret
	secrets map[string]map[string]string
}

func (gh GitHubSecret) githubSecret(secretKey string) string {
	secret := commonconfig.GetString(gh.s.Ref, "")
	return gh.secrets[secret][secretKey]
}

func (gh GitHubSecret) AccessTokenKey() string {
	key := commonconfig.GetString(gh.s.AccessTokenKey, "")
	return gh.githubSecret(key)
}

type NotificationsConfig struct {
	c       toolchainv1alpha1.NotificationsConfig
	secrets map[string]map[string]string
}

func (n NotificationsConfig) notificationSecret(secretKey string) string {
	secret := commonconfig.GetString(n.c.Secret.Ref, "")
	return n.secrets[secret][secretKey]
}

func (n NotificationsConfig) NotificationDeliveryService() string {
	return commonconfig.GetString(n.c.NotificationDeliveryService, "mailgun")
}

func (n NotificationsConfig) DurationBeforeNotificationDeletion() time.Duration {
	v := commonconfig.GetString(n.c.DurationBeforeNotificationDeletion, "24h")
	duration, err := time.ParseDuration(v)
	if err != nil {
		duration = 24 * time.Hour
	}
	return duration
}

func (n NotificationsConfig) AdminEmail() string {
	return commonconfig.GetString(n.c.AdminEmail, "")
}

func (n NotificationsConfig) TemplateSetName() string {
	return commonconfig.GetString(n.c.TemplateSetName, "sandbox")
}

func (n NotificationsConfig) MailgunDomain() string {
	key := commonconfig.GetString(n.c.Secret.MailgunDomain, "")
	return n.notificationSecret(key)
}

func (n NotificationsConfig) MailgunAPIKey() string {
	key := commonconfig.GetString(n.c.Secret.MailgunAPIKey, "")
	return n.notificationSecret(key)
}

func (n NotificationsConfig) MailgunSenderEmail() string {
	key := commonconfig.GetString(n.c.Secret.MailgunSenderEmail, "")
	return n.notificationSecret(key)
}

func (n NotificationsConfig) MailgunReplyToEmail() string {
	key := commonconfig.GetString(n.c.Secret.MailgunReplyToEmail, "")
	return n.notificationSecret(key)
}

type RegistrationServiceConfig struct {
	c toolchainv1alpha1.RegistrationServiceConfig
}

func (r RegistrationServiceConfig) Environment() string {
	return commonconfig.GetString(r.c.Environment, "prod")
}

func (r RegistrationServiceConfig) Replicas() int32 {
	return commonconfig.GetInt32(r.c.Replicas, 3)
}

func (r RegistrationServiceConfig) Analytics() AnalyticsConfig {
	return AnalyticsConfig{r.c.Analytics}
}

type AnalyticsConfig struct {
	c toolchainv1alpha1.RegistrationServiceAnalyticsConfig
}

func (r AnalyticsConfig) SegmentWriteKey() string {
	return commonconfig.GetString(r.c.SegmentWriteKey, "")
}

func (r RegistrationServiceConfig) RegistrationServiceURL() string {
	return commonconfig.GetString(r.c.RegistrationServiceURL, "https://registration.crt-placeholder.com")
}

type TiersConfig struct {
	tiers toolchainv1alpha1.TiersConfig
}

func (d TiersConfig) DefaultUserTier() string {
	return commonconfig.GetString(d.tiers.DefaultUserTier, "deactivate30")
}

func (d TiersConfig) DefaultSpaceTier() string {
	return commonconfig.GetString(d.tiers.DefaultSpaceTier, "base")
}

func (d TiersConfig) DurationBeforeChangeTierRequestDeletion() time.Duration {
	v := commonconfig.GetString(d.tiers.DurationBeforeChangeTierRequestDeletion, "24h")
	duration, err := time.ParseDuration(v)
	if err != nil {
		duration = 24 * time.Hour
	}
	return duration
}

type ToolchainStatusConfig struct {
	t toolchainv1alpha1.ToolchainStatusConfig
}

func (d ToolchainStatusConfig) ToolchainStatusRefreshTime() time.Duration {
	v := commonconfig.GetString(d.t.ToolchainStatusRefreshTime, "5s")
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
	return commonconfig.GetInt(d.c.MasterUserRecordUpdateFailureThreshold, 2) // default: allow 1 failure, try again and then give up if failed again
}

func (d UsersConfig) ForbiddenUsernamePrefixes() []string {
	prefixes := commonconfig.GetString(d.c.ForbiddenUsernamePrefixes, "openshift,kube,default,redhat,sandbox")
	v := strings.FieldsFunc(prefixes, func(c rune) bool {
		return c == ','
	})
	return v
}

func (d UsersConfig) ForbiddenUsernameSuffixes() []string {
	suffixes := commonconfig.GetString(d.c.ForbiddenUsernameSuffixes, "admin")
	v := strings.FieldsFunc(suffixes, func(c rune) bool {
		return c == ','
	})
	return v
}
