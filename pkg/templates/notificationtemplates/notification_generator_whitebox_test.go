package notificationtemplates

import (
	"testing"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestLoadTemplates(t *testing.T) {
	logf.SetLogger(zap.Logger(true))

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			assets := NewAssets(AssetNames, Asset)

			// when
			tmpls, err := loadTemplates(assets)

			// then
			require.NoError(t, err)
			require.Len(t, tmpls, 1)

			for _, notificationType := range []string{"userprovisioned"} {
				templates := tmpls[notificationType]
				require.Len(t, templates, 2)
				for _, template := range templates {
					require.NotEmpty(t, template.templateName)
					require.NotEmpty(t, template.content)
				}
			}
		})
	})
}

func TestNewTemplates(t *testing.T) {
	logf.SetLogger(zap.Logger(true))

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			assets := NewAssets(AssetNames, Asset)
			namespace := "host-operator-" + uuid.NewV4().String()[:7]
			// when
			tmpls, err := loadTemplates(assets)
			require.NoError(t, err)
			notificationTemplates, err := newTemplates(namespace, tmpls)

			// then
			require.NoError(t, err)
			require.NotEmpty(t, notificationTemplates)
			for _, template := range notificationTemplates {
				assert.Equal(t, namespace, template.Namespace)
				assert.NotEmpty(t, template.Spec.Content)
				assert.NotEmpty(t, template.Spec.Subject)
			}
		})
	})
}
