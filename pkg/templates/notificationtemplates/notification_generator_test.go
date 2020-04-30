package notificationtemplates_test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestGetNotificationTemplates(t *testing.T) {
	logf.SetLogger(zap.Logger(true))
	t.Run("ok", func(t *testing.T) {
		t.Run("get notification templates", func(t *testing.T) {
			// when
			template, found, err := notificationtemplates.GetNotificationTemplates("userprovisioned", notificationtemplates.WithEmptyCache())
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.True(t, found)
			assert.NotEmpty(t, template.Content)
			assert.NotEmpty(t, template.Subject)
			assert.Equal(t, template.Subject, "Notice: Your Red Hat CodeReady Toolchain account has been provisioned")
			assert.Contains(t, template.Content, "You are receiving this email because you have an online <a href={{.RegistrationURL}}>Red Hat CodeReady Toolchain</a>")
		})
		t.Run("ensure cache is used", func(t *testing.T) {
			// when
			_, _, err := notificationtemplates.GetNotificationTemplates("userprovisioned", notificationtemplates.WithEmptyCache())
			require.NoError(t, err)
			template, found, err := notificationtemplates.GetNotificationTemplates("userprovisioned")
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.True(t, found)
			assert.NotEmpty(t, template.Content)
			assert.NotEmpty(t, template.Subject)
			assert.Equal(t, template.Subject, "Notice: Your Red Hat CodeReady Toolchain account has been provisioned")
			assert.Contains(t, template.Content, "You are receiving this email because you have an online <a href={{.RegistrationURL}}>Red Hat CodeReady Toolchain</a>")
		})
	})
	t.Run("failures", func(t *testing.T) {
		t.Run("failed to get notification templates", func(t *testing.T) {
			// given
			fakeAssets := notificationtemplates.NewAssets(notificationtemplates.AssetNames, func(name string) ([]byte, error) {
				// error occurs when fetching the content of the a notification template
				return nil, errors.Errorf("an error")
			})

			// when
			template, found, err := notificationtemplates.GetNotificationTemplates("userprovisioned", notificationtemplates.WithAssets(fakeAssets))
			// then
			require.Error(t, err)
			require.False(t, found)
			assert.Nil(t, template)
			assert.Equal(t, "unable to get notification templates: an error", err.Error())
		})
		t.Run("no filename", func(t *testing.T) {
			// given
			fakeAssets := notificationtemplates.NewAssets(func() []string {
				// error occurs when fetching the content of the a notification template
				return []string{"test"}
			}, func(s string) (bytes []byte, err error) {
				return bytes, err
			})

			// when
			template, found, err := notificationtemplates.GetNotificationTemplates("userprovisioned", notificationtemplates.WithAssets(fakeAssets))
			// then
			require.Error(t, err)
			require.False(t, found)
			assert.Nil(t, template)
			assert.Equal(t, "unable to get notification templates: unable to load templates: path must contain directory and file", err.Error())
		})
		t.Run("non-existent notification template", func(t *testing.T) {
			// given
			fakeAssets := notificationtemplates.NewAssets(func() []string {
				// error occurs when fetching the content of the a notification template
				return []string{"test/test"}
			}, func(s string) (bytes []byte, err error) {
				return bytes, err
			})

			// when
			template, found, err := notificationtemplates.GetNotificationTemplates("userprovisioned", notificationtemplates.WithAssets(fakeAssets))
			// then
			require.Error(t, err)
			require.False(t, found)
			assert.Nil(t, template)
			assert.Equal(t, "unable to get notification templates: unable to load templates: must contain notification.html and subject.txt", err.Error())
		})
	})
}
