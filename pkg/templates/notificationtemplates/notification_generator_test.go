package notificationtemplates_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	testsupport "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestCreateOrUpdateResources(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	logf.SetLogger(zap.Logger(true))

	t.Run("ok", func(t *testing.T) {
		t.Run("create only", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			clt := testsupport.NewFakeClient(t)
			// verify that notification template resources exist prior to creation
			for _, templateName := range []string{"userprovisioned"} {
				template := toolchainv1alpha1.NotificationTemplate{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: templateName}, &template)
				require.Error(t, err)
				assert.IsType(t, metav1.StatusReasonNotFound, apierrors.ReasonForError(err))
			}
			assets := notificationtemplates.NewAssets(notificationtemplates.AssetNames, notificationtemplates.Asset)

			// when
			err := notificationtemplates.CreateOrUpdateResources(s, clt, namespace, assets)

			// then
			require.NoError(t, err)

			// verify that notification template resources exist after creation
			for _, templateName := range []string{"userprovisioned"} {
				template := toolchainv1alpha1.NotificationTemplate{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: templateName}, &template)
				require.NoError(t, err)
				assert.NotEmpty(t, template.Spec.Subject)
				assert.NotEmpty(t, template.Spec.Content)
			}
		})
	})
	t.Run("failures", func(t *testing.T) {

		namespace := "host-operator" + uuid.NewV4().String()[:7]
		clt := testsupport.NewFakeClient(t)

		t.Run("failed to generate notification temaplte", func(t *testing.T) {
			// given
			fakeAssets := notificationtemplates.NewAssets(notificationtemplates.AssetNames, func(name string) ([]byte, error) {
				// error occurs when fetching the content of the a notification template
				return nil, errors.Errorf("an error")
			})
			// when
			err := notificationtemplates.CreateOrUpdateResources(s, clt, namespace, fakeAssets)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to create or update NotificationTemplate: unable to load templates: an error", err.Error())
		})

		t.Run("failed to update notification template", func(t *testing.T) {
			// given
			// initialize the client with an existing userprovisioned notification template
			clt := testsupport.NewFakeClient(t, &toolchainv1alpha1.NotificationTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "userprovisioned",
				},
			})
			clt.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				// trigger an error when trying to update the existing userprovisioned notification template
				return errors.Errorf("an error")
			}
			assets := notificationtemplates.NewAssets(notificationtemplates.AssetNames, notificationtemplates.Asset)
			// when
			err := notificationtemplates.CreateOrUpdateResources(s, clt, namespace, assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("unable to create or update the 'userprovisioned' NotificationTemplate in namespace '%s'", namespace))
		})
	})
}
