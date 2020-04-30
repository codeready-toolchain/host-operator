package notificationtemplates_test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"

	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestCreateOrUpdateResources(t *testing.T) {
	logf.SetLogger(zap.Logger(true))
	t.Run("ok", func(t *testing.T) {
		t.Run("get notification templates", func(t *testing.T) {
			// given
			assets := notificationtemplates.NewAssets(notificationtemplates.AssetNames, notificationtemplates.Asset)

			// when
			template, err := notificationtemplates.GetNotificationTemplates("userprovisioned", assets)
			// then
			require.NoError(t, err)
			require.NotNil(t, template)
			assert.NotEmpty(t, template.Content)
			assert.NotEmpty(t, template.Subject)
			assert.Equal(t, template.Subject, "Notice: Your Red Hat CodeReady Toolchain account has been provisioned")
		})
	})
	t.Run("failures", func(t *testing.T) {
		namespace := "host-operator" + uuid.NewV4().String()[:7]
		t.Run("failed to get notification templates", func(t *testing.T) {
			// given
			fakeAssets := notificationtemplates.NewAssets(notificationtemplates.AssetNames, func(name string) ([]byte, error) {
				// error occurs when fetching the content of the a notification template
				return nil, errors.Errorf("an error")
			})
			// when
			template, err := notificationtemplates.GetNotificationTemplates(namespace, fakeAssets)
			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "unable to get notification templates: an error", err.Error())
		})
	})
	t.Run("failures", func(t *testing.T) {
		namespace := "host-operator" + uuid.NewV4().String()[:7]
		t.Run("no filename", func(t *testing.T) {
			// given
			fakeAssets := notificationtemplates.NewAssets(func() []string {
				// error occurs when fetching the content of the a notification template
				return []string{"test"}
			}, func(s string) (bytes []byte, err error) {
				return bytes, err
			})

			// when
			template, err := notificationtemplates.GetNotificationTemplates(namespace, fakeAssets)
			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "unable to get notification templates: unable to load templates: path must contain directory and file", err.Error())
		})
	})
	t.Run("failures", func(t *testing.T) {
		t.Run("no directory or filename", func(t *testing.T) {
			// given
			fakeAssets := notificationtemplates.NewAssets(func() []string {
				// error occurs when fetching the content of the a notification template
				return []string{"/"}
			}, func(s string) (bytes []byte, err error) {
				return bytes, err
			})

			// when
			template, err := notificationtemplates.GetNotificationTemplates("userprovisioned", fakeAssets)
			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "unable to get notification templates: unable to load templates: directory name and filename cannot be empty", err.Error())
		})
	})
	t.Run("failures", func(t *testing.T) {
		t.Run("non-existent notification template", func(t *testing.T) {
			// given
			fakeAssets := notificationtemplates.NewAssets(func() []string {
				// error occurs when fetching the content of the a notification template
				return []string{"test/test"}
			}, func(s string) (bytes []byte, err error) {
				return bytes, err
			})

			// when
			template, err := notificationtemplates.GetNotificationTemplates("userprovisioned", fakeAssets)
			// then
			require.Error(t, err)
			assert.Nil(t, template)
			assert.Equal(t, "unable to get notification templates: unable to load templates: must contain notification.html and subject.txt", err.Error())
		})
	})
}
