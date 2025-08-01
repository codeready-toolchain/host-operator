package toolchainconfig

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testconfig "github.com/codeready-toolchain/toolchain-common/pkg/test/config"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
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
		require.NoError(t, err)
		assert.Equal(t, "prod", toolchainCfg.Environment())
		assert.Empty(t, toolchainCfg.Notifications().MailgunAPIKey())
	})

	t.Run("config object retrieved", func(t *testing.T) {
		// given
		commonconfig.ResetCache()
		toolchainCfgObj := testconfig.NewToolchainConfigObj(t, testconfig.Environment("e2e-tests"), testconfig.Notifications().Secret().Ref("notifications-secret").MailgunAPIKey("mailgunAPIKey"), testconfig.Notifications().TemplateSetName("sandbox"))
		secret := test.CreateSecret("notifications-secret", test.HostOperatorNs, map[string][]byte{
			"mailgunAPIKey": []byte("abc123"),
		})
		cl := test.NewFakeClient(t, toolchainCfgObj, secret)

		// when
		toolchainCfg, err := GetToolchainConfig(cl)

		// then
		require.NoError(t, err)
		assert.Equal(t, "e2e-tests", toolchainCfg.Environment())
		assert.Equal(t, "abc123", toolchainCfg.Notifications().MailgunAPIKey())
		assert.Equal(t, "sandbox", toolchainCfg.Notifications().TemplateSetName())

		t.Run("config object updated but cache not updated", func(t *testing.T) {
			// given
			obj := testconfig.ModifyToolchainConfigObj(t, cl, testconfig.Environment("unit-tests"), testconfig.Notifications().TemplateSetName("appstudio"))
			err := cl.Update(context.TODO(), obj)
			require.NoError(t, err)

			secret.Data["mailgunAPIKey"] = []byte("def456")
			err = cl.Update(context.TODO(), secret)
			require.NoError(t, err)

			actual := &toolchainv1alpha1.ToolchainConfig{}
			err = cl.Get(context.TODO(), types.NamespacedName{Name: "config", Namespace: test.HostOperatorNs}, actual)
			require.NoError(t, err)
			assert.Equal(t, "unit-tests", *actual.Spec.Host.Environment)

			// when
			toolchainCfg, err = GetToolchainConfig(cl)

			// then
			require.NoError(t, err)
			assert.Equal(t, "e2e-tests", toolchainCfg.Environment()) // returns cached value
			assert.Equal(t, "abc123", toolchainCfg.Notifications().MailgunAPIKey())
			assert.Equal(t, "sandbox", toolchainCfg.Notifications().TemplateSetName())
		})
	})

	t.Run("error retrieving config object", func(t *testing.T) {
		// given
		commonconfig.ResetCache()
		toolchainCfgObj := testconfig.NewToolchainConfigObj(t, testconfig.Environment("e2e-tests"))
		cl := test.NewFakeClient(t, toolchainCfgObj)
		cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return fmt.Errorf("get failed")
		}

		// when
		toolchainCfg, err := GetToolchainConfig(cl)

		// then
		require.EqualError(t, err, "get failed")
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
		require.NoError(t, err)
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
		require.NoError(t, err)
		assert.Equal(t, "e2e-tests", toolchainCfg.Environment())
		assert.Equal(t, "abc123", toolchainCfg.Notifications().MailgunAPIKey())

		t.Run("config object updated but cache not updated", func(t *testing.T) {
			// given
			obj := testconfig.ModifyToolchainConfigObj(t, cl, testconfig.Environment("unit-tests"))
			err := cl.Update(context.TODO(), obj)
			require.NoError(t, err)

			secret.Data["mailgunAPIKey"] = []byte("def456")
			err = cl.Update(context.TODO(), secret)
			require.NoError(t, err)

			actual := &toolchainv1alpha1.ToolchainConfig{}
			err = cl.Get(context.TODO(), types.NamespacedName{Name: "config", Namespace: test.HostOperatorNs}, actual)
			require.NoError(t, err)
			assert.Equal(t, "unit-tests", *actual.Spec.Host.Environment)

			// when
			toolchainCfg, err = ForceLoadToolchainConfig(cl)

			// then
			require.NoError(t, err)
			assert.Equal(t, "unit-tests", toolchainCfg.Environment())               // returns actual value
			assert.Equal(t, "def456", toolchainCfg.Notifications().MailgunAPIKey()) // returns actual value
		})
	})

	t.Run("error retrieving config object", func(t *testing.T) {
		// given
		commonconfig.ResetCache()
		toolchainCfgObj := testconfig.NewToolchainConfigObj(t, testconfig.Environment("e2e-tests"))
		cl := test.NewFakeClient(t, toolchainCfgObj)
		cl.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return fmt.Errorf("get failed")
		}

		// when
		toolchainCfg, err := ForceLoadToolchainConfig(cl)

		// then
		require.EqualError(t, err, "get failed")
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
	})
	t.Run("non-default no domains", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().Enabled(true))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.True(t, toolchainCfg.AutomaticApproval().IsEnabled())
	})

	t.Run("non-default single domain", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().
			Enabled(true).Domains("somedomain.org"))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.True(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Equal(t, []string{"somedomain.org"}, toolchainCfg.AutomaticApproval().Domains())
	})

	t.Run("non-default multiple domains", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().
			Enabled(true).Domains("somedomain.org,otherdomain.edu"))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.True(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Equal(t, []string{"somedomain.org", "otherdomain.edu"}, toolchainCfg.AutomaticApproval().Domains())
	})

	t.Run("non-default multiple domains with spaces", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().
			Enabled(true).Domains(" somedomain.org, otherdomain.edu "))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.True(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Equal(t, []string{"somedomain.org", "otherdomain.edu"}, toolchainCfg.AutomaticApproval().Domains())
	})

	t.Run("non-default empty domains", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.AutomaticApproval().
			Enabled(true).Domains(""))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.True(t, toolchainCfg.AutomaticApproval().IsEnabled())
		assert.Empty(t, toolchainCfg.AutomaticApproval().Domains())
	})
}

func TestDeactivationConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, 3, toolchainCfg.Deactivation().DeactivatingNotificationDays())
		assert.Empty(t, toolchainCfg.Deactivation().DeactivationDomainsExcluded())
		assert.Equal(t, 1460, toolchainCfg.Deactivation().UserSignupDeactivatedRetentionDays())
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
		assert.Equal(t, "sandbox", toolchainCfg.Notifications().TemplateSetName()) // default notificationEnv
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.Notifications().
				AdminEmail("joe.schmoe@redhat.com").
				TemplateSetName("appstudio").
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
		assert.Equal(t, "appstudio", toolchainCfg.Notifications().TemplateSetName())
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
		assert.Equal(t, "https://registration.crt-placeholder.com", toolchainCfg.RegistrationService().RegistrationServiceURL())
		assert.Equal(t, int32(3), toolchainCfg.RegistrationService().Replicas())
		assert.False(t, toolchainCfg.RegistrationService().Verification().CaptchaEnabled())
		assert.Empty(t, toolchainCfg.RegistrationService().Verification().CaptchaServiceAccountFileContents())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.RegistrationService().
			Environment("e2e-tests").
			RegistrationServiceURL("https://registration.crt.com").
			Replicas(2).
			Namespace("another-namespace").
			Verification().CaptchaEnabled(true).
			Verification().Secret().Ref("verification-secrets").
			RecaptchaServiceAccountFile("captcha.json"))

		verificationSecretValues := make(map[string]string)
		verificationSecretValues["captcha.json"] = "example-content"
		secrets := make(map[string]map[string]string)
		secrets["verification-secrets"] = verificationSecretValues

		toolchainCfg := newToolchainConfig(cfg, secrets)

		assert.Equal(t, "e2e-tests", toolchainCfg.RegistrationService().Environment())
		assert.Equal(t, "https://registration.crt.com", toolchainCfg.RegistrationService().RegistrationServiceURL())
		assert.Equal(t, int32(2), toolchainCfg.RegistrationService().Replicas())
		assert.True(t, toolchainCfg.RegistrationService().Verification().CaptchaEnabled())
		assert.Equal(t, "example-content", toolchainCfg.RegistrationService().Verification().CaptchaServiceAccountFileContents())
	})
}

func TestTiers(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, "deactivate30", toolchainCfg.Tiers().DefaultUserTier())
		assert.Equal(t, "base", toolchainCfg.Tiers().DefaultSpaceTier())
		assert.Equal(t, 24*time.Hour, toolchainCfg.Tiers().DurationBeforeChangeTierRequestDeletion())
		assert.Empty(t, toolchainCfg.Tiers().FeatureToggles())
	})
	t.Run("invalid", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Tiers().DurationBeforeChangeTierRequestDeletion("rapid"))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, 24*time.Hour, toolchainCfg.Tiers().DurationBeforeChangeTierRequestDeletion())
	})
	t.Run("non-default", func(t *testing.T) {
		weight10 := uint(10)
		cfg := commonconfig.NewToolchainConfigObjWithReset(t, testconfig.Tiers().
			DefaultUserTier("deactivate90").
			DefaultSpaceTier("ourtier").
			DurationBeforeChangeTierRequestDeletion("48h").
			FeatureToggle("feature-1", nil). // With default weight
			FeatureToggle("feature-2", &weight10))
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Equal(t, "deactivate90", toolchainCfg.Tiers().DefaultUserTier())
		assert.Equal(t, "ourtier", toolchainCfg.Tiers().DefaultSpaceTier())
		assert.Equal(t, 48*time.Hour, toolchainCfg.Tiers().DurationBeforeChangeTierRequestDeletion())
		assert.Len(t, toolchainCfg.Tiers().FeatureToggles(), 2)
		assert.Equal(t, "feature-1", toolchainCfg.Tiers().FeatureToggles()[0].Name())
		assert.Equal(t, uint(100), toolchainCfg.Tiers().FeatureToggles()[0].Weight()) // default weight
		assert.Equal(t, "feature-2", toolchainCfg.Tiers().FeatureToggles()[1].Name())
		assert.Equal(t, weight10, toolchainCfg.Tiers().FeatureToggles()[1].Weight())
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

func TestGitHubSecret(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.Empty(t, toolchainCfg.GitHubSecret().AccessTokenKey())
	})
	t.Run("non-default", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t,
			testconfig.ToolchainStatus().
				GitHubSecretRef("github").
				GitHubSecretAccessTokenKey("accessToken"))
		gitHubSecretValues := make(map[string]string)
		gitHubSecretValues["accessToken"] = "abc123"
		secrets := make(map[string]map[string]string)
		secrets["github"] = gitHubSecretValues

		toolchainCfg := newToolchainConfig(cfg, secrets)

		assert.Equal(t, "abc123", toolchainCfg.GitHubSecret().AccessTokenKey())
	})
}

func TestPublicViewer(t *testing.T) {
	t.Run("not-set", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.False(t, toolchainCfg.PublicViewer().Enabled())
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		cfg.Spec.Host.PublicViewerConfig = &toolchainv1alpha1.PublicViewerConfiguration{Enabled: false}
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.False(t, toolchainCfg.PublicViewer().Enabled())
	})

	t.Run("enabled", func(t *testing.T) {
		cfg := commonconfig.NewToolchainConfigObjWithReset(t)
		cfg.Spec.Host.PublicViewerConfig = &toolchainv1alpha1.PublicViewerConfiguration{Enabled: true}
		toolchainCfg := newToolchainConfig(cfg, map[string]map[string]string{})

		assert.True(t, toolchainCfg.PublicViewer().Enabled())
	})
}
