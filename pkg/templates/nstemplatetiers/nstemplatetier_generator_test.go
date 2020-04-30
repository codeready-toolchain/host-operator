package nstemplatetiers_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"
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
			// set the 'generation' when creating the object
			clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
				if obj, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					if obj.ObjectMeta.Generation != 0 {
						return errors.Errorf("'generation' field will be specified by the server during object creation: %v", obj.ObjectMeta.Generation)
					}
					obj.ObjectMeta.Generation = obj.ObjectMeta.Generation + 1
				}
				return clt.Client.Create(ctx, obj)
			}
			// verify that no "advanced" nor "basic" nor "team" NSTemplateTier resources exist prior to creation
			for _, tierName := range []string{"advanced", "basic", "team"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.Error(t, err)
				assert.IsType(t, metav1.StatusReasonNotFound, apierrors.ReasonForError(err))
			}
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)

			// then
			require.NoError(t, err)
			// verify that 4 NSTemplateTier CRs were created: "advanced", "basic", "team", "nocluster"
			for _, tierName := range []string{"advanced", "basic", "team", "nocluster"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(1), tier.ObjectMeta.Generation)
				if tier.Name == "nocluster" {
					assert.Nil(t, tier.Spec.ClusterResources) // "team" tier should not have cluster resources set
				} else {
					assert.NotNil(t, tier.Spec.ClusterResources)
				}
			}
		})

		t.Run("create and update", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			clt := testsupport.NewFakeClient(t)
			// set the 'generation' when creating the object
			clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
				if tier, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					if tier.ObjectMeta.Generation != 0 {
						return errors.Errorf("'generation' field will be specified by the server during object creation: %v", tier.ObjectMeta.Generation)
					}
					tier.ObjectMeta.Generation = tier.ObjectMeta.Generation + 1
					tier.ObjectMeta.ResourceVersion = "foo" // set by the server
					err := clt.Client.Create(ctx, obj)
					if err != nil && apierrors.IsAlreadyExists(err) {
						// prevent the fake client to return the actual ResourceVersion,
						// as it seems like the real client/server don't not do it in the e2e tests
						// see client.MockUpdate just underneath for the use-case
						tier.ResourceVersion = ""
					}
					return err
				}
				return errors.Errorf("did not expect to create a resource of type %T", obj)
			}
			// check the 'generation' when updating the object, and increment its value
			clt.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				if obj, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
					if obj.ObjectMeta.ResourceVersion != "foo" {
						// here, we expect that the caller will have set the ResourceVersion based on the existing object
						return errors.Errorf("'ResourceVersion ' field must be specified during object update")
					}
					// increment even if the object did not change
					existing := toolchainv1alpha1.NSTemplateTier{}
					if err := clt.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, &existing); err != nil {
						return err
					}
					obj.SetGeneration(existing.GetGeneration() + 1)
				}
				return clt.Client.Update(ctx, obj)
			}
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)
			require.NoError(t, err)
			// verify that 4 NSTemplateTier CRs were created: "advanced", "basic", "team", "nocluster"
			for _, tierName := range []string{"advanced", "basic", "team", "nocluster"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				// verify that the ResourceVersion was set by the server
				assert.Equal(t, "foo", tier.ObjectMeta.ResourceVersion)
				// verify that the generation was set to 1
				assert.Equal(t, int64(1), tier.ObjectMeta.Generation)
				// verify their namespace revisions
				for _, ns := range tier.Spec.Namespaces {
					assert.Equal(t, nstemplatetiers.ExpectedRevisions[tierName][ns.Type], ns.Revision)
				}
			}

			// when calling CreateOrUpdateResources a second time
			err = nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)

			// then
			require.NoError(t, err)
			// verify that the 4 NStemplateTier CRs were updated
			for _, tierName := range []string{"advanced", "basic", "team", "nocluster"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				// verify that the generation was increased
				assert.Equal(t, int64(2), tier.ObjectMeta.Generation)
				// verify their new namespace revisions
				for _, ns := range tier.Spec.Namespaces {
					assert.Equal(t, nstemplatetiers.ExpectedRevisions[tierName][ns.Type], ns.Revision)
					assert.NotEmpty(t, nstemplatetiers.ExpectedRevisions[tierName][ns.Type], ns.Template)
				}
			}
		})
	})

	t.Run("failures", func(t *testing.T) {

		namespace := "host-operator" + uuid.NewV4().String()[:7]
		clt := testsupport.NewFakeClient(t)

		t.Run("failed to generate nstemplatetiers", func(t *testing.T) {
			// given
			fakeAssets := assets.NewAssets(testnstemplatetiers.AssetNames, func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte("advanced-code: abcdef"), nil
				}
				// error occurs when fetching the content of the 'advanced-code.yaml' template
				return nil, errors.Errorf("an error")
			})
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, fakeAssets)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to create or update NSTemplateTiers: unable to load templates: an error", err.Error())
		})

		t.Run("failed to create nstemplatetiers", func(t *testing.T) {
			// given
			clt := testsupport.NewFakeClient(t)
			clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
				// simulate a client/server error
				return errors.Errorf("an error")
			}
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("unable to create or update the 'advanced' NSTemplateTiers in namespace '%s'", namespace))
		})

		t.Run("failed to update nstemplatetiers", func(t *testing.T) {
			// given
			// initialize the client with an existing `advanced` NSTemplatetier
			clt := testsupport.NewFakeClient(t, &toolchainv1alpha1.NSTemplateTier{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "advanced",
				},
			})
			clt.MockUpdate = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				// trigger an error when trying to update the existing `advanced` NSTemplatetier
				return errors.Errorf("an error")
			}
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("unable to create or update the 'advanced' NSTemplateTiers in namespace '%s'", namespace))
		})

	})
}
