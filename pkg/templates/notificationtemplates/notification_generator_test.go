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
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			assets := notificationtemplates.NewAssets(notificationtemplates.AssetNames, notificationtemplates.Asset)

			// when
			templates, err := notificationtemplates.GetNotificationTemplates(namespace, assets)

			// then
			require.NoError(t, err)
			require.NotNil(t, templates)

			// verify that notification template resources exist after creation
			for _, template := range templates {
				assert.Equal(t, template.Name, "userprovisioned")
				assert.NotEmpty(t, template.Spec.Subject)
				assert.NotEmpty(t, template.Spec.Content)
			}
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
			_, err := notificationtemplates.GetNotificationTemplates(namespace, fakeAssets)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to get notification templates: unable to load templates: an error", err.Error())
		})
	})
}
