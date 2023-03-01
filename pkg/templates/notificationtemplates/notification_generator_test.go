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
	t.Run("ok", func(t *testing.T) {
		t.Run("get userdeactivated notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, found, err := GetNotificationTemplate("userdeactivated", sandboxNotificationEnvironment)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.True(t, found)
			assert.Equal(t, "Notice: Your Developer Sandbox for Red Hat OpenShift account is deactivated", template.Subject)
			assert.Contains(t, template.Content, "Your account is now deactivated and all your data on Developer Sandbox for Red Hat OpenShift has been deleted.")
		})
		t.Run("get userprovisioned notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, found, err := GetNotificationTemplate("userprovisioned", sandboxNotificationEnvironment)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.True(t, found)
			assert.Equal(t, "Notice: Your Developer Sandbox for Red Hat OpenShift account is provisioned", template.Subject)
			assert.Contains(t, template.Content, "Your account has been provisioned and is ready to use. Your account will be active for 30 days.")
		})
		t.Run("ensure cache is used", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			_, _, err := GetNotificationTemplate("userprovisioned", sandboxNotificationEnvironment)
			require.NoError(t, err)
			template, err := loadTemplates("sandbox")
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			require.NotEmpty(t, template["userprovisioned"])
			assert.Equal(t, "Notice: Your Developer Sandbox for Red Hat OpenShift account is provisioned", template["userprovisioned"].Subject)
			assert.Contains(t, template["userprovisioned"].Content, "Your account has been provisioned and is ready to use. Your account will be active for 30 days.")
			assert.Equal(t, template["userprovisioned"], *SandboxUserProvisioned)
		})
		t.Run("get userdeactivating notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, found, err := GetNotificationTemplate("userdeactivating", sandboxNotificationEnvironment)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.True(t, found)
			assert.Equal(t, "Notice: Your Developer Sandbox for Red Hat OpenShift account will be deactivated soon", template.Subject)
			assert.Contains(t, template.Content, "Your sandbox will expire in 3 days.  We recommend you save your work as all data in your sandbox will be\n        deleted upon expiry.")

		})

		t.Run("get idlertriggered notification template", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, found, err := GetNotificationTemplate("idlertriggered", sandboxNotificationEnvironment)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.True(t, found)
			assert.Equal(t, "Notice: Your running application in namespace {{.Namespace}} has been idled", template.Subject)
			assert.Contains(t, template.Content, "In accordance with the usage terms of Developer Sandbox, we have reduced the number of instances of your\n        application to zero (0).")

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
			template, err := templatesForAssets(fakeTemplates, "testTemplates", sandboxNotificationEnvironment)
			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "unable to load templates: path must contain directory and file", err.Error())
		})
		t.Run("non-existent notification template", func(t *testing.T) {
			// given
			defer resetNotificationTemplateCache()
			// when
			template, err := templatesForAssets(fakeTemplates, "testTemplates", stonesoupNotificationEnvironment)
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
