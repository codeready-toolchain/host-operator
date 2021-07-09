package toolchainconfig_test

import (
	"testing"
	"time"

	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
)

func TestEnvironment(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t)
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, "prod", toolchainCfg.Environment())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t, testconfig.Environment(testconfig.E2E))
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, "e2e-tests", toolchainCfg.Environment())
	})
}

func TestAutomaticApprovalConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t)
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.False(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Equal(t, 1000, toolchainCfg.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Empty(t, toolchainCfg.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		assert.Equal(t, 80, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdDefault())
		assert.Empty(t, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t, testconfig.AutomaticApproval().Enabled(true).MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)).ResourceCapacityThreshold(456, testconfig.PerMemberCluster("member1", 654)))
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.True(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Equal(t, 123, toolchainCfg.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Equal(t, cfg.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, toolchainCfg.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		assert.Equal(t, 456, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdDefault())
		assert.Equal(t, cfg.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.SpecificPerMemberCluster, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	})
}

func TestDeactivationConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t)
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, 3, toolchainCfg.Deactivation().DeactivatingNotificationDays())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t, testconfig.Deactivation().DeactivatingNotificationDays(5))
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, 5, toolchainCfg.Deactivation().DeactivatingNotificationDays())
	})
}

func TestMetrics(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t)
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.False(t, toolchainCfg.Metrics().ForceSynchronization())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t, testconfig.Metrics().ForceSynchronization(true))
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.True(t, toolchainCfg.Metrics().ForceSynchronization())
	})
}

func TestNotifications(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t)
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Empty(t, toolchainCfg.Notifications().AdminEmail())
		assert.Empty(t, toolchainCfg.Notifications().MailgunDomain())
		assert.Empty(t, toolchainCfg.Notifications().MailgunAPIKey())
		assert.Empty(t, toolchainCfg.Notifications().MailgunSenderEmail())
		assert.Empty(t, toolchainCfg.Notifications().MailgunReplyToEmail())
		assert.Equal(t, "mailgun", toolchainCfg.Notifications().NotificationDeliveryService())
		assert.Equal(t, 24*time.Hour, toolchainCfg.Notifications().DurationBeforeNotificationDeletion())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t,
			testconfig.Notifications().
				AdminEmail("joe.schmoe@redhat.com").
				DurationBeforeNotificationDeletion("48h").
				NotificationDeliveryService("mailknife").
				Secret().
				Ref("notifications").
				MailgunAPIKey("mailgunAPIKey").
				MailgunDomain("mailgunDomain").
				MailgunReplyToEmail("replyTo").
				MailgunSenderEmail("sender"))
		notificationSecretValues := make(map[string]string)
		notificationSecretValues["mailgunAPIKey"] = "abc123"
		notificationSecretValues["mailgunDomain"] = "domain.abc"
		notificationSecretValues["replyTo"] = "devsandbox_rulez@redhat.com"
		notificationSecretValues["sender"] = "devsandbox@redhat.com"
		secrets := make(map[string]map[string]string)
		secrets["notifications"] = notificationSecretValues

		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, secrets)

		assert.Equal(t, "joe.schmoe@redhat.com", toolchainCfg.Notifications().AdminEmail())
		assert.Equal(t, "abc123", toolchainCfg.Notifications().MailgunAPIKey())
		assert.Equal(t, "domain.abc", toolchainCfg.Notifications().MailgunDomain())
		assert.Equal(t, "devsandbox_rulez@redhat.com", toolchainCfg.Notifications().MailgunReplyToEmail())
		assert.Equal(t, "devsandbox@redhat.com", toolchainCfg.Notifications().MailgunSenderEmail())
		assert.Equal(t, "mailknife", toolchainCfg.Notifications().NotificationDeliveryService())
		assert.Equal(t, 48*time.Hour, toolchainCfg.Notifications().DurationBeforeNotificationDeletion())
	})
}

func TestRegistrationService(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t)
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, "https://registration.crt-placeholder.com", toolchainCfg.RegistrationService().RegistrationServiceURL())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t, testconfig.RegistrationService().RegistrationServiceURL("www.crtregservice.com"))
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, "www.crtregservice.com", toolchainCfg.RegistrationService().RegistrationServiceURL())
	})
}

func TestTiers(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t)
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, 24*time.Hour, toolchainCfg.Tiers().DurationBeforeChangeTierRequestDeletion())
		assert.Equal(t, 5, toolchainCfg.Tiers().TemplateUpdateRequestMaxPoolSize())
	})
	t.Run("invalid", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t, testconfig.Tiers().DurationBeforeChangeTierRequestDeletion("rapid"))
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, 24*time.Hour, toolchainCfg.Tiers().DurationBeforeChangeTierRequestDeletion())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t, testconfig.Tiers().DurationBeforeChangeTierRequestDeletion("48h").TemplateUpdateRequestMaxPoolSize(40))
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, 48*time.Hour, toolchainCfg.Tiers().DurationBeforeChangeTierRequestDeletion())
		assert.Equal(t, 40, toolchainCfg.Tiers().TemplateUpdateRequestMaxPoolSize())
	})
}

func TestToolchainStatus(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t)
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, 5*time.Second, toolchainCfg.ToolchainStatus().ToolchainStatusRefreshTime())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t, testconfig.ToolchainStatus().ToolchainStatusRefreshTime("10s"))
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, 10*time.Second, toolchainCfg.ToolchainStatus().ToolchainStatusRefreshTime())
	})
}

func TestUsers(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t)
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, 2, toolchainCfg.Users().MasterUserRecordUpdateFailureThreshold())
		assert.Equal(t, []string{"openshift", "kube", "default", "redhat", "sandbox"}, toolchainCfg.Users().ForbiddenUsernamePrefixes())
		assert.Equal(t, []string{"admin"}, toolchainCfg.Users().ForbiddenUsernameSuffixes())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := toolchainconfig.NewToolchainConfigWithReset(t, testconfig.Users().MasterUserRecordUpdateFailureThreshold(10).ForbiddenUsernamePrefixes("bread,butter").ForbiddenUsernameSuffixes("sugar,cream"))
		toolchainCfg := toolchainconfig.NewToolchainConfig(&cfg.Spec, map[string]map[string]string{})

		assert.Equal(t, 10, toolchainCfg.Users().MasterUserRecordUpdateFailureThreshold())
		assert.Equal(t, []string{"bread", "butter"}, toolchainCfg.Users().ForbiddenUsernamePrefixes())
		assert.Equal(t, []string{"sugar", "cream"}, toolchainCfg.Users().ForbiddenUsernameSuffixes())
	})
}
