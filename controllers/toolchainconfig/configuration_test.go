package toolchainconfig

import (
	"context"
	"fmt"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/assert"
)

func TestGetToolchainConfig(t *testing.T) {
	restore := test.SetEnvVarAndRestore(t, "WATCH_NAMESPACE", test.HostOperatorNs)
	defer restore()
	t.Run("no config object - default config", func(t *testing.T) {
		// given
		commonconfig.ResetCache()
		cl := test.NewFakeClient(t)

		// when
		toolchainCfg, err := GetToolchainConfig(cl)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "prod", toolchainCfg.Environment())
		assert.Empty(t, toolchainCfg.Notifications().MailgunAPIKey())
	})

	t.Run("config object retrieved", func(t *testing.T) {
		// given
		commonconfig.ResetCache()
		toolchainCfgObj := testconfig.NewToolchainConfigObj(t, testconfig.Environment("e2e-tests"), testconfig.Notifications().Secret().Ref("notifications-secret").MailgunAPIKey("mailgunAPIKey"))
		secret := test.CreateSecret("notifications-secret", test.HostOperatorNs, map[string][]byte{
			"mailgunAPIKey": []byte("abc123"),
		})
		cl := test.NewFakeClient(t, toolchainCfgObj, secret)

		// when
		toolchainCfg, err := GetToolchainConfig(cl)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "e2e-tests", toolchainCfg.Environment())
		assert.Equal(t, "abc123", toolchainCfg.Notifications().MailgunAPIKey())

		t.Run("config object updated but cache not updated", func(t *testing.T) {
			// given
			obj := testconfig.ModifyToolchainConfigObj(t, cl, testconfig.Environment("unit-tests"))
			err := cl.Update(context.TODO(), obj)
			assert.NoError(t, err)

			secret.Data["mailgunAPIKey"] = []byte("def456")
			err = cl.Update(context.TODO(), secret)
			assert.NoError(t, err)

			actual := &toolchainv1alpha1.ToolchainConfig{}
			err = cl.Get(context.TODO(), types.NamespacedName{Name: "config", Namespace: test.HostOperatorNs}, actual)
			assert.NoError(t, err)
			assert.Equal(t, "unit-tests", *actual.Spec.Host.Environment)

			// when
			toolchainCfg, err = GetToolchainConfig(cl)

			// then
			assert.NoError(t, err)
			assert.Equal(t, "e2e-tests", toolchainCfg.Environment()) // returns cached value
			assert.Equal(t, "abc123", toolchainCfg.Notifications().MailgunAPIKey())
		})
	})

	t.Run("error retrieving config object", func(t *testing.T) {
		// given
		commonconfig.ResetCache()
		toolchainCfgObj := testconfig.NewToolchainConfigObj(t, testconfig.Environment("e2e-tests"))
		cl := test.NewFakeClient(t, toolchainCfgObj)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			return fmt.Errorf("get failed")
		}

		// when
		toolchainCfg, err := GetToolchainConfig(cl)

		// then
		assert.EqualError(t, err, "get failed")
		assert.Equal(t, "prod", toolchainCfg.Environment())
		assert.Empty(t, toolchainCfg.Notifications().MailgunAPIKey())
	})
}

func TestGetCachedToolchainConfig(t *testing.T) {
	t.Run("cache uninitialized", func(t *testing.T) {
		// when
		toolchainCfg := GetCachedToolchainConfig()

		// then
		assert.Equal(t, "prod", toolchainCfg.Environment())
		assert.Empty(t, toolchainCfg.Notifications().MailgunAPIKey())
	})

	t.Run("cache initialized", func(t *testing.T) {
		// given
		toolchainCfgObj := testconfig.NewToolchainConfigObj(t, testconfig.Environment("e2e-tests"), testconfig.Notifications().Secret().Ref("notifications-secret").MailgunAPIKey("mailgunAPIKey"))
		commonconfig.UpdateConfig(toolchainCfgObj, map[string]map[string]string{
			"notifications-secret": {
				"mailgunAPIKey": "abc123",
			},
		})

		// when
		toolchainCfg := GetCachedToolchainConfig()

		// then
		assert.Equal(t, "e2e-tests", toolchainCfg.Environment())
		assert.Equal(t, "abc123", toolchainCfg.Notifications().MailgunAPIKey())
	})
}

func TestForceLoadToolchainConfig(t *testing.T) {
	restore := test.SetEnvVarAndRestore(t, "WATCH_NAMESPACE", test.HostOperatorNs)
	defer restore()
	t.Run("no config object - default config", func(t *testing.T) {
		// given
		commonconfig.ResetCache()
		cl := test.NewFakeClient(t)

		// when
		toolchainCfg, err := ForceLoadToolchainConfig(cl)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "prod", toolchainCfg.Environment())
		assert.Empty(t, toolchainCfg.Notifications().MailgunAPIKey())
	})

	t.Run("config object retrieved", func(t *testing.T) {
		// given
		commonconfig.ResetCache()
		toolchainCfgObj := testconfig.NewToolchainConfigObj(t, testconfig.Environment("e2e-tests"), testconfig.Notifications().Secret().Ref("notifications-secret").MailgunAPIKey("mailgunAPIKey"))
		secret := test.CreateSecret("notifications-secret", test.HostOperatorNs, map[string][]byte{
			"mailgunAPIKey": []byte("abc123"),
		})
		cl := test.NewFakeClient(t, toolchainCfgObj, secret)

		// when
		toolchainCfg, err := ForceLoadToolchainConfig(cl)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "e2e-tests", toolchainCfg.Environment())
		assert.Equal(t, "abc123", toolchainCfg.Notifications().MailgunAPIKey())

		t.Run("config object updated but cache not updated", func(t *testing.T) {
			// given
			obj := testconfig.ModifyToolchainConfigObj(t, cl, testconfig.Environment("unit-tests"))
			err := cl.Update(context.TODO(), obj)
			assert.NoError(t, err)

			secret.Data["mailgunAPIKey"] = []byte("def456")
			err = cl.Update(context.TODO(), secret)
			assert.NoError(t, err)

			actual := &toolchainv1alpha1.ToolchainConfig{}
			err = cl.Get(context.TODO(), types.NamespacedName{Name: "config", Namespace: test.HostOperatorNs}, actual)
			assert.NoError(t, err)
			assert.Equal(t, "unit-tests", *actual.Spec.Host.Environment)

			// when
			toolchainCfg, err = ForceLoadToolchainConfig(cl)

			// then
			assert.NoError(t, err)
			assert.Equal(t, "unit-tests", toolchainCfg.Environment())               // returns actual value
			assert.Equal(t, "def456", toolchainCfg.Notifications().MailgunAPIKey()) // returns actual value
		})

	})

	t.Run("error retrieving config object", func(t *testing.T) {
		// given
		commonconfig.ResetCache()
		toolchainCfgObj := testconfig.NewToolchainConfigObj(t, testconfig.Environment("e2e-tests"))
		cl := test.NewFakeClient(t, toolchainCfgObj)
		cl.MockGet = func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			return fmt.Errorf("get failed")
		}

		// when
		toolchainCfg, err := ForceLoadToolchainConfig(cl)

		// then
		assert.EqualError(t, err, "get failed")
		assert.Equal(t, "prod", toolchainCfg.Environment())
		assert.Empty(t, toolchainCfg.Notifications().MailgunAPIKey())
	})
}

func TestNewToolchainConfig(t *testing.T) {
	t.Run("correct object type - config initialized correctly", func(t *testing.T) {
		// given
		toolchainCfgObj := testconfig.NewToolchainConfigObj(t, testconfig.Environment("e2e-tests"))

		// when
		toolchainCfg := newToolchainConfig(toolchainCfgObj, nil)

		// then
		assert.Equal(t, "e2e-tests", toolchainCfg.Environment())
	})

	t.Run("incorrect object type - default config used", func(t *testing.T) {
		// given
		toolchainCfgObj := testconfig.NewMemberOperatorConfigObj()

		// when
		toolchainCfg := newToolchainConfig(toolchainCfgObj, nil)

		// then
		assert.Equal(t, "prod", toolchainCfg.Environment())
	})

	t.Run("nil toolchainconfig resource - default config used", func(t *testing.T) {
		// when
		toolchainCfg := newToolchainConfig(nil, nil)

		// then
		assert.Equal(t, "prod", toolchainCfg.Environment())
	})
}

func TestAutomaticApprovalConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.False(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Equal(t, 1000, toolchainCfg.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Empty(t, toolchainCfg.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		assert.Equal(t, 80, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdDefault())
		assert.Empty(t, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true).MaxNumberOfUsers(123, testconfig.PerMemberCluster("member1", 321)).ResourceCapacityThreshold(456, testconfig.PerMemberCluster("member1", 654)))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.True(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Equal(t, 123, toolchainCfg.AutomaticApproval().MaxNumberOfUsersOverall())
		assert.Equal(t, cfg.Spec.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster, toolchainCfg.AutomaticApproval().MaxNumberOfUsersSpecificPerMemberCluster())
		assert.Equal(t, 456, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdDefault())
		assert.Equal(t, cfg.Spec.Host.AutomaticApproval.ResourceCapacityThreshold.SpecificPerMemberCluster, toolchainCfg.AutomaticApproval().ResourceCapacityThresholdSpecificPerMemberCluster())
	})
}

func TestDeactivationConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, 3, toolchainCfg.Deactivation().DeactivatingNotificationDays())
		assert.Empty(t, toolchainCfg.Deactivation().DeactivationDomainsExcluded())
		assert.Equal(t, 365, toolchainCfg.Deactivation().UserSignupDeactivatedRetentionDays())
		assert.Equal(t, 7, toolchainCfg.Deactivation().UserSignupUnverifiedRetentionDays())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Deactivation().
			DeactivatingNotificationDays(5).
			DeactivationDomainsExcluded("@redhat.com,@ibm.com").
			UserSignupDeactivatedRetentionDays(44).
			UserSignupUnverifiedRetentionDays(77))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, 5, toolchainCfg.Deactivation().DeactivatingNotificationDays())
		assert.Equal(t, []string{"@redhat.com", "@ibm.com"}, toolchainCfg.Deactivation().DeactivationDomainsExcluded())
		assert.Equal(t, 44, toolchainCfg.Deactivation().UserSignupDeactivatedRetentionDays())
		assert.Equal(t, 77, toolchainCfg.Deactivation().UserSignupUnverifiedRetentionDays())
	})
}

func TestEnvironment(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, "prod", toolchainCfg.Environment())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Environment(testconfig.E2E))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, "e2e-tests", toolchainCfg.Environment())
	})
}

func TestMetrics(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.False(t, toolchainCfg.Metrics().ForceSynchronization())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Metrics().ForceSynchronization(true))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.True(t, toolchainCfg.Metrics().ForceSynchronization())
	})
}

func TestNotifications(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Empty(t, toolchainCfg.Notifications().AdminEmail())
		assert.Empty(t, toolchainCfg.Notifications().MailgunDomain())
		assert.Empty(t, toolchainCfg.Notifications().MailgunAPIKey())
		assert.Empty(t, toolchainCfg.Notifications().MailgunSenderEmail())
		assert.Empty(t, toolchainCfg.Notifications().MailgunReplyToEmail())
		assert.Equal(t, "mailgun", toolchainCfg.Notifications().NotificationDeliveryService())
		assert.Equal(t, 24*time.Hour, toolchainCfg.Notifications().DurationBeforeNotificationDeletion())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t,
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

		toolchainCfg := newToolchainConfig(cfg, secrets)

		assert.Equal(t, "joe.schmoe@redhat.com", toolchainCfg.Notifications().AdminEmail())
		assert.Equal(t, "abc123", toolchainCfg.Notifications().MailgunAPIKey())
		assert.Equal(t, "domain.abc", toolchainCfg.Notifications().MailgunDomain())
		assert.Equal(t, "devsandbox_rulez@redhat.com", toolchainCfg.Notifications().MailgunReplyToEmail())
		assert.Equal(t, "devsandbox@redhat.com", toolchainCfg.Notifications().MailgunSenderEmail())
		assert.Equal(t, "mailknife", toolchainCfg.Notifications().NotificationDeliveryService())
		assert.Equal(t, 48*time.Hour, toolchainCfg.Notifications().DurationBeforeNotificationDeletion())
	})

	t.Run("edge case", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.Notifications().
				DurationBeforeNotificationDeletion("banana"))

		toolchainCfg := newToolchainConfig(cfg, nil)

		assert.Equal(t, 24*time.Hour, toolchainCfg.Notifications().DurationBeforeNotificationDeletion())
	})
}

func TestRegistrationService(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, "prod", toolchainCfg.RegistrationService().Environment())
		assert.Equal(t, "toolchain-host-operator", toolchainCfg.RegistrationService().Namespace())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.RegistrationService().
			Environment("e2e-tests").
			Namespace("another-namespace"))

		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, "e2e-tests", toolchainCfg.RegistrationService().Environment())
		assert.Equal(t, "another-namespace", toolchainCfg.RegistrationService().Namespace())
	})
}

func TestTiers(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, "base", toolchainCfg.Tiers().DefaultTier())
		assert.Equal(t, 24*time.Hour, toolchainCfg.Tiers().DurationBeforeChangeTierRequestDeletion())
		assert.Equal(t, 5, toolchainCfg.Tiers().TemplateUpdateRequestMaxPoolSize())
	})
	t.Run("invalid", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Tiers().DurationBeforeChangeTierRequestDeletion("rapid"))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, 24*time.Hour, toolchainCfg.Tiers().DurationBeforeChangeTierRequestDeletion())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Tiers().
			DefaultTier("advanced").
			DurationBeforeChangeTierRequestDeletion("48h").
			TemplateUpdateRequestMaxPoolSize(40))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, "advanced", toolchainCfg.Tiers().DefaultTier())
		assert.Equal(t, 48*time.Hour, toolchainCfg.Tiers().DurationBeforeChangeTierRequestDeletion())
		assert.Equal(t, 40, toolchainCfg.Tiers().TemplateUpdateRequestMaxPoolSize())
	})
}

func TestToolchainStatus(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, 5*time.Second, toolchainCfg.ToolchainStatus().ToolchainStatusRefreshTime())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.ToolchainStatus().ToolchainStatusRefreshTime("10s"))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, 10*time.Second, toolchainCfg.ToolchainStatus().ToolchainStatusRefreshTime())
	})
	t.Run("edge case", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.ToolchainStatus().ToolchainStatusRefreshTime("banana"))

		toolchainCfg := newToolchainConfig(cfg, nil)

		assert.Equal(t, 5*time.Second, toolchainCfg.ToolchainStatus().ToolchainStatusRefreshTime())
	})
}

func TestUsers(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, 2, toolchainCfg.Users().MasterUserRecordUpdateFailureThreshold())
		assert.Equal(t, []string{"openshift", "kube", "default", "redhat", "sandbox"}, toolchainCfg.Users().ForbiddenUsernamePrefixes())
		assert.Equal(t, []string{"admin"}, toolchainCfg.Users().ForbiddenUsernameSuffixes())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Users().MasterUserRecordUpdateFailureThreshold(10).ForbiddenUsernamePrefixes("bread,butter").ForbiddenUsernameSuffixes("sugar,cream"))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, 10, toolchainCfg.Users().MasterUserRecordUpdateFailureThreshold())
		assert.Equal(t, []string{"bread", "butter"}, toolchainCfg.Users().ForbiddenUsernamePrefixes())
		assert.Equal(t, []string{"sugar", "cream"}, toolchainCfg.Users().ForbiddenUsernameSuffixes())
	})
}
