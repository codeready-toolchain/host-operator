package notificationtemplates

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestGetNotificationTemplate(t *testing.T) {
	logf.SetLogger(zap.Logger(true))
	t.Run("ok", func(t *testing.T) {
		t.Run("get notification templates", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			template, found, err := GetNotificationTemplate("userprovisioned")
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.True(t, found)
			assert.Equal(t, "Notice: Your Red Hat CodeReady Toolchain account has been provisioned", template.Subject)
			assert.Contains(t, template.Content, "You are receiving this email because you have an online <a href={{.RegistrationURL}}>Red Hat CodeReady Toolchain</a>")
		})
		t.Run("ensure cache is used", func(t *testing.T) {
			// when
			defer resetNotificationTemplateCache()
			_, _, err := GetNotificationTemplate("userprovisioned")
			require.NoError(t, err)
			template, err := loadTemplates()
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			require.NotEmpty(t, template["userprovisioned"])
			assert.Equal(t, "Notice: Your Red Hat CodeReady Toolchain account has been provisioned", template["userprovisioned"].Subject)
			assert.Contains(t, template["userprovisioned"].Content, "You are receiving this email because you have an online <a href={{.RegistrationURL}}>Red Hat CodeReady Toolchain</a>")
			assert.Equal(t, template["userprovisioned"], *UserProvisioned)
		})
	})
	t.Run("failures", func(t *testing.T) {
		t.Run("failed to get notification templates", func(t *testing.T) {
			// given
			defer resetNotificationTemplateCache()
			fakeAssets := assets.NewAssets(AssetNames, func(name string) ([]byte, error) {
				// error occurs when fetching the content of the a notification template
				return nil, errors.Errorf("an error")
			})

			// when
			template, err := templatesForAssets(fakeAssets)

			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "an error", err.Error())
		})
		t.Run("no filename", func(t *testing.T) {
			// given
			defer resetNotificationTemplateCache()
			fakeAssets := assets.NewAssets(func() []string {
				// error occurs when fetching the content of the a notification template
				return []string{"test"}
			}, func(s string) (bytes []byte, err error) {
				return bytes, err
			})

			// when
			template, err := templatesForAssets(fakeAssets)
			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "unable to load templates: path must contain directory and file", err.Error())
		})
		t.Run("non-existent notification template", func(t *testing.T) {
			// given
			defer resetNotificationTemplateCache()
			fakeAssets := assets.NewAssets(func() []string {
				// error occurs when fetching the content of the a notification template
				return []string{"test/test"}
			}, func(s string) (bytes []byte, err error) {
				return bytes, err
			})

			// when
			template, err := templatesForAssets(fakeAssets)
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
