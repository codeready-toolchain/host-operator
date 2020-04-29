package nstemplatetiers_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
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
	mockCreate := func(clt *testsupport.FakeClient) func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
		return func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
			var objMeta *metav1.ObjectMeta
			switch obj := obj.(type) {
			case *toolchainv1alpha1.NSTemplateTier:
				objMeta = &obj.ObjectMeta
			case *toolchainv1alpha1.TierTemplate:
				objMeta = &obj.ObjectMeta
			default:
				return errors.Errorf("did not expect to create a resource of type %T", obj)
			}
			if objMeta.Generation != 0 {
				return errors.Errorf("'generation' field will be specified by the server during object creation: %v", objMeta.Generation)
			}
			objMeta.Generation = 1
			objMeta.ResourceVersion = "foo" // set by the server
			err := clt.Client.Create(ctx, obj)
			if err != nil && apierrors.IsAlreadyExists(err) {
				// prevent the fake client to return the actual ResourceVersion,
				// as it seems like the real client/server don't not do it in the e2e tests
				// see client.MockUpdate just underneath for the use-case
				objMeta.ResourceVersion = ""
			}
			return err
		}
	}

	// check the 'generation' when updating the object, and increment its value
	mockUpdate := func(clt *testsupport.FakeClient) func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
		return func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
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
				return clt.Client.Update(ctx, obj)
			}
			return errors.Errorf("did not expect to update a resource of type %T", obj)
		}
	}

	assets := nstemplatetiers.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)

	t.Run("ok", func(t *testing.T) {

		t.Run("create only", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			clt := testsupport.NewFakeClient(t)
			clt.MockCreate = mockCreate(clt)
			clt.MockUpdate = mockUpdate(clt)
			// verify that no NSTemplateTier resources exist prior to creation
			nsTmplTiers := toolchainv1alpha1.NSTemplateTierList{}
			err = clt.List(context.TODO(), &nsTmplTiers, client.InNamespace(namespace))
			require.NoError(t, err)
			require.Empty(t, nsTmplTiers.Items)
			// verify that no TierTemplate resources exist prior to creation
			tierTmpls := toolchainv1alpha1.TierTemplateList{}
			err = clt.List(context.TODO(), &tierTmpls, client.InNamespace(namespace))
			require.NoError(t, err)
			require.Empty(t, tierTmpls.Items)

			assets := nstemplatetiers.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)

			// then
			require.NoError(t, err)

			// verify that 14 TierTemplates were created (see `testnstemplatetiers.AssetNames`)
			tierTmpls = toolchainv1alpha1.TierTemplateList{}
			err = clt.List(context.TODO(), &tierTmpls, client.InNamespace(namespace))
			require.NoError(t, err)
			require.Len(t, tierTmpls.Items, len(testnstemplatetiers.AssetNames())-1) // exclude `metadata.yml` from the AssetNames, it does not result in a TemplateTier resource
			names := make([]string, len(testnstemplatetiers.AssetNames())-1)
			for i, tierTmpl := range tierTmpls.Items {
				names[i] = tierTmpl.Name
			}
			assert.ElementsMatch(t, []string{
				"advanced-clusterresources-654321a",
				"advanced-code-123456a",
				"advanced-dev-123456b",
				"advanced-stage-123456c",
				"basic-clusterresources-654321b",
				"basic-code-123456d",
				"basic-dev-123456e",
				"basic-stage-123456f",
				"team-clusterresources-654321c",
				"team-dev-123456g",
				"team-stage-123456h",
				"nocluster-code-123456i",
				"nocluster-dev-123456j",
				"nocluster-stage-1234567",
			}, names)

			// verify that 4 NSTemplateTier CRs were created: "advanced", "basic", "team", "nocluster"
			for _, tierName := range []string{"advanced", "basic", "team", "nocluster"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(1), tier.ObjectMeta.Generation)
				if tier.Name == "nocluster" {
					assert.Nil(t, tier.Spec.ClusterResources) // "team" tier should not have cluster resources set
				} else {
					require.NotNil(t, tier.Spec.ClusterResources)
					assert.Equal(t, tier.Name+"-clusterresources-"+tier.Spec.ClusterResources.Revision, tier.Spec.ClusterResources.TemplateRef)
				}
				for _, ns := range tier.Spec.Namespaces {
					assert.Equal(t, nstemplatetiers.ExpectedRevisions[tierName][ns.Type], ns.Revision)
					assert.NotEmpty(t, nstemplatetiers.ExpectedRevisions[tierName][ns.Type], ns.Template)
					assert.Equal(t, tier.Name+"-"+ns.Type+"-"+ns.Revision, ns.TemplateRef)
				}
			}
		})

		t.Run("create", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			clt := testsupport.NewFakeClient(t)
			clt.MockCreate = mockCreate(clt)
			clt.MockUpdate = mockUpdate(clt)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)
			require.NoError(t, err)

			t.Run("then update", func(t *testing.T) {

				// when calling CreateOrUpdateResources a second time
				err = nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)

				// then
				require.NoError(t, err)
				// verify that all TemplateTier CRs were updated
				tierTmpls := toolchainv1alpha1.TierTemplateList{}
				err = clt.List(context.TODO(), &tierTmpls, client.InNamespace(namespace))
				require.NoError(t, err)
				require.Len(t, tierTmpls.Items, len(testnstemplatetiers.AssetNames())-1) // exclude `metadata.yml` from the AssetNames, it does not result in a TemplateTier resource
				for _, tierTmpl := range tierTmpls.Items {
					assert.Equal(t, int64(1), tierTmpl.ObjectMeta.Generation) // unchanged
				}

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
						assert.Equal(t, tier.Name+"-"+ns.Type+"-"+ns.Revision, ns.TemplateRef)
					}
				}
			})
		})
	})

	t.Run("failures", func(t *testing.T) {

		namespace := "host-operator" + uuid.NewV4().String()[:7]

		t.Run("failed to read assets", func(t *testing.T) {
			// given
			clt := testsupport.NewFakeClient(t)
			fakeAssets := nstemplatetiers.NewAssets(testnstemplatetiers.AssetNames, func(name string) ([]byte, error) {
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

		t.Run("nstemplatetiers", func(t *testing.T) {

			t.Run("failed to create nstemplatetiers", func(t *testing.T) {
				// given
				clt := testsupport.NewFakeClient(t)
				clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Create(ctx, obj, opts...)
				}
				assets := nstemplatetiers.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
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
					if _, ok := obj.(*toolchainv1alpha1.NSTemplateTier); ok {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Update(ctx, obj, opts...)
				}
				assets := nstemplatetiers.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
				// when
				err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)
				// then
				require.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("unable to create or update the 'advanced' NSTemplateTiers in namespace '%s'", namespace))
			})
		})

		t.Run("tiertemplates", func(t *testing.T) {

			t.Run("failed to create nstemplatetiers", func(t *testing.T) {
				// given
				clt := testsupport.NewFakeClient(t)
				clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.TierTemplate); ok {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Create(ctx, obj, opts...)
				}
				assets := nstemplatetiers.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
				// when
				err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)
				// then
				require.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("unable to create or update the 'advanced-code-123456a' TierTemplate in namespace '%s'", namespace))
			})
		})

	})
}
