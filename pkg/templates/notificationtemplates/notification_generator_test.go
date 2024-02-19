package notificationtemplates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestGetNotificationTemplate(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	t.Run("sandbox ok", func(t *testing.T) {
		t.Run("get userdeactivated notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate(UserDeactivatedTemplateName, SandboxTemplateSetName)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.Equal(t, "Notice: Your Developer Sandbox for Red Hat OpenShift account is deactivated", template.Subject)
			assert.Contains(t, template.Content, "Your account is now deactivated and all your data on Developer Sandbox for Red Hat OpenShift has been deleted.")
		})
		t.Run("get userprovisioned notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate(UserProvisionedTemplateName, SandboxTemplateSetName)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.Equal(t, "Notice: Your Developer Sandbox for Red Hat OpenShift account is provisioned", template.Subject)
			assert.Contains(t, template.Content, "Your account has been provisioned and is ready to use. Your account will be active for 30 days.")
		})
		t.Run("ensure cache is used", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			_, err := GetNotificationTemplate(UserProvisionedTemplateName, SandboxTemplateSetName)
			require.NoError(t, err)
			template, err := loadTemplates("sandbox")
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			require.NotEmpty(t, template["userprovisioned"])
			assert.Equal(t, "Notice: Your Developer Sandbox for Red Hat OpenShift account is provisioned", template["userprovisioned"].Subject)
			assert.Contains(t, template["userprovisioned"].Content, "Your account has been provisioned and is ready to use. Your account will be active for 30 days.")
			assert.Equal(t, template["userprovisioned"].Name, UserProvisionedTemplateName)
		})
		t.Run("get userdeactivating notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate(UserDeactivatingTemplateName, SandboxTemplateSetName)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.Equal(t, "Notice: Your Developer Sandbox for Red Hat OpenShift account will be deactivated soon", template.Subject)
			assert.Contains(t, template.Content, "Your sandbox will expire in 3 days.  We recommend you save your work as all data in your sandbox will be\n        deleted upon expiry.")

		})

		t.Run("get idlertriggered notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate(IdlerTriggeredTemplateName, SandboxTemplateSetName)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.Equal(t, "Notice: Your running workload in namespace {{.Namespace}} has been idled", template.Subject)
			assert.Contains(t, template.Content, "In accordance with the usage terms of Developer Sandbox, your workload {{.AppType}} {{.AppName}} has been scaled down.")

		})
	})

	t.Run("appstudio ok", func(t *testing.T) {
		t.Run("get userprovisioned notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate(UserProvisionedTemplateName, AppstudioTemplateSetName)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.Equal(t, "Welcome to Red Hat Trusted Application Pipeline!", template.Subject)
			assert.Contains(t, template.Content, "Welcome to Red Hat Trusted Application Pipeline")
			assert.NotContains(t, template.Content, "Sandbox")
			assert.Equal(t, UserProvisionedTemplateName, template.Name)
		})
		t.Run("get userdeactivating notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate(UserDeactivatingTemplateName, AppstudioTemplateSetName)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.Equal(t, "Notice: Your RHTAP account will be deactivated soon", template.Subject)
			assert.Contains(t, template.Content, "The Red Hat Trusted Application Pipeline team")
			assert.NotContains(t, template.Content, "Sandbox")
			assert.Equal(t, UserDeactivatingTemplateName, template.Name)
		})
		t.Run("get userdeactivated notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate(UserDeactivatedTemplateName, AppstudioTemplateSetName)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.Equal(t, "Notice: Your RHTAP account is deactivated", template.Subject)
			assert.Contains(t, template.Content, "The Red Hat Trusted Application Pipeline team")
			assert.NotContains(t, template.Content, "Sandbox")
			assert.Equal(t, UserDeactivatedTemplateName, template.Name)
		})
		t.Run("get idlertriggered notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate(IdlerTriggeredTemplateName, AppstudioTemplateSetName)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.Equal(t, "Notice: Your running application has been idled", template.Subject)
			assert.Contains(t, template.Content, "The Red Hat Trusted Application Pipeline team")
			assert.NotContains(t, template.Content, "Sandbox")
			assert.Equal(t, IdlerTriggeredTemplateName, template.Name)
		})
	})

	t.Run("sandbox not ok", func(t *testing.T) {
		t.Run("wrong template name return empty template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate("test", SandboxTemplateSetName)
			// then
			require.Error(t, err)
			assert.Equal(t, "notification template test not found in sandbox", err.Error())
			require.Empty(t, template)

		})
	})

	t.Run("appstudio not ok", func(t *testing.T) {
		t.Run("wrong template name return empty template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, err := GetNotificationTemplate("test", AppstudioTemplateSetName)
			// then
			require.Error(t, err)
			assert.Equal(t, "notification template test not found in appstudio", err.Error())
			require.Empty(t, template)
		})
	})
}

func TestTemplatesForAssets(t *testing.T) {

	t.Run("failures", func(t *testing.T) {
		t.Run("failed to get notification templates for a non existent environment", func(t *testing.T) {
			// given
			defer resetNotificationTemplateCache()
			// when
			template, err := templatesForAssets(fakeTemplates, "testTemplates", "alpha")

			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "Could not find any emails templates for the environment alpha", err.Error())
		})
		t.Run("no directory name", func(t *testing.T) {
			// given
			defer resetNotificationTemplateCache()
			// when
			template, err := templatesForAssets(fakeTemplates, "testTemplates", SandboxTemplateSetName)
			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "unable to load templates: path must contain directory and file", err.Error())
		})
		t.Run("non-existent notification template", func(t *testing.T) {
			// given
			defer resetNotificationTemplateCache()
			// when
			template, err := templatesForAssets(fakeTemplates, "testTemplates", AppstudioTemplateSetName)
			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "unable to load templates: must contain notification.html and subject.txt", err.Error())
		})
	})
}

func resetNotificationTemplateCache() {
	notificationTemplates = nil
}
