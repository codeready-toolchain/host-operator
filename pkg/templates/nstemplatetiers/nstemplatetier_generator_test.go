package nstemplatetiers_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/gofrs/uuid"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"
	testsupport "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	testassets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)

	t.Run("ok", func(t *testing.T) {

		expectedTemplateRefs := map[string]map[string]interface{}{
			"advanced": {
				"clusterresources": "advanced-clusterresources-abcd123-654321a",
				"namespaces": []string{
					"advanced-dev-abcd123-123456b",
					"advanced-stage-abcd123-123456c",
				},
				"spaceRoles": map[string]string{
					"admin": "advanced-admin-abcd123-123456d",
				},
			},
			"base": {
				"clusterresources": "base-clusterresources-654321a-654321a",
				"namespaces": []string{
					"base-dev-123456b-123456b",
					"base-stage-123456c-123456c",
				},
				"spaceRoles": map[string]string{
					"admin": "base-admin-123456d-123456d",
				},
			},
			"nocluster": {
				"namespaces": []string{
					"nocluster-dev-123456j-123456j",
					"nocluster-stage-1234567-1234567",
				},
				"spaceRoles": map[string]string{
					"admin": "nocluster-admin-123456k-123456k",
				},
			},
			"appstudio": {
				"clusterresources": "appstudio-clusterresources-654321a-654321a",
				"namespaces": []string{
					"appstudio-appstudio-123456b-123456b",
				},
				"spaceRoles": map[string]string{
					"admin":  "appstudio-admin-123456c-123456c",
					"viewer": "appstudio-viewer-123456d-123456d",
				},
			},
		}

		t.Run("create only", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.Must(uuid.NewV4()).String()[:7]
			clt := testsupport.NewFakeClient(t)
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

			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)

			// then
			require.NoError(t, err)

			// verify that TierTemplates were created
			tierTmpls = toolchainv1alpha1.TierTemplateList{}
			err = clt.List(context.TODO(), &tierTmpls, client.InNamespace(namespace))
			require.NoError(t, err)
			require.Len(t, tierTmpls.Items, 15) // 4 items for advanced and base tiers + 3 for nocluster tier + 4 for appstudio
			names := []string{}
			for _, tierTmpl := range tierTmpls.Items {
				names = append(names, tierTmpl.Name)
			}
			require.ElementsMatch(t, []string{
				"advanced-clusterresources-abcd123-654321a",
				"advanced-dev-abcd123-123456b",
				"advanced-stage-abcd123-123456c",
				"advanced-admin-abcd123-123456d",
				"base-clusterresources-654321a-654321a",
				"base-dev-123456b-123456b",
				"base-stage-123456c-123456c",
				"base-admin-123456d-123456d",
				"nocluster-dev-123456j-123456j",
				"nocluster-stage-1234567-1234567",
				"nocluster-admin-123456k-123456k",
				"appstudio-clusterresources-654321a-654321a",
				"appstudio-appstudio-123456b-123456b",
				"appstudio-admin-123456c-123456c",
				"appstudio-viewer-123456d-123456d",
			}, names)

			// verify that 4 NSTemplateTier CRs were created:
			for _, tierName := range []string{"advanced", "base", "nocluster", "appstudio"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(1), tier.ObjectMeta.Generation)

				// check the `clusterresources` templateRef
				if tier.Name == "nocluster" {
					assert.Nil(t, tier.Spec.ClusterResources) // "nocluster" tier should not have cluster resources set
				} else {
					require.NotNil(t, tier.Spec.ClusterResources)
					assert.Equal(t, expectedTemplateRefs[tierName]["clusterresources"], tier.Spec.ClusterResources.TemplateRef)
				}

				// check the `namespaces` templateRefs
				actualNamespaceTmplRefs := make([]string, len(tier.Spec.Namespaces))
				for i, ns := range tier.Spec.Namespaces {
					actualNamespaceTmplRefs[i] = ns.TemplateRef
				}
				assert.ElementsMatch(t, expectedTemplateRefs[tierName]["namespaces"], actualNamespaceTmplRefs)

				// check the `spaceRoles` templateRefs
				actualSpaceRoleTmplRefs := make(map[string]string, len(tier.Spec.SpaceRoles))
				for i, r := range tier.Spec.SpaceRoles {
					actualSpaceRoleTmplRefs[i] = r.TemplateRef
				}
				for role, tmpl := range expectedTemplateRefs[tierName]["spaceRoles"].(map[string]string) {
					assert.Equal(t, tmpl, actualSpaceRoleTmplRefs[role])
				}
			}
		})

		t.Run("create then update with same tier templates", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.Must(uuid.NewV4()).String()[:7]
			clt := testsupport.NewFakeClient(t)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testassets)
			require.NoError(t, err)

			// when calling CreateOrUpdateResources a second time
			err = nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testassets)

			// then
			require.NoError(t, err)
			// verify that all TierTemplate CRs were updated
			tierTmpls := toolchainv1alpha1.TierTemplateList{}
			err = clt.List(context.TODO(), &tierTmpls, client.InNamespace(namespace))
			require.NoError(t, err)
			require.Len(t, tierTmpls.Items, 15) // 4 items for advanced and base tiers + 3 for nocluster tier + 4 for appstudio
			for _, tierTmpl := range tierTmpls.Items {
				assert.Equal(t, int64(1), tierTmpl.ObjectMeta.Generation) // unchanged
			}

			// verify that 4 NSTemplateTier CRs were created:
			for _, tierName := range []string{"advanced", "base", "nocluster", "appstudio"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(1), tier.ObjectMeta.Generation)

				// check the `clusterresources` templateRef
				if tier.Name == "nocluster" {
					assert.Nil(t, tier.Spec.ClusterResources)
				} else {
					require.NotNil(t, tier.Spec.ClusterResources)
					assert.Equal(t, expectedTemplateRefs[tierName]["clusterresources"], tier.Spec.ClusterResources.TemplateRef)
				}

				// check the `namespaces` templateRefs
				actualTemplateRefs := make([]string, len(tier.Spec.Namespaces))
				for i, ns := range tier.Spec.Namespaces {
					actualTemplateRefs[i] = ns.TemplateRef
				}
				assert.ElementsMatch(t, expectedTemplateRefs[tierName]["namespaces"], actualTemplateRefs)

				// check the `spaceRoles` templateRefs
				actualSpaceRoleTmplRefs := make(map[string]string, len(tier.Spec.SpaceRoles))
				for i, r := range tier.Spec.SpaceRoles {
					actualSpaceRoleTmplRefs[i] = r.TemplateRef
				}
				for role, tmpl := range expectedTemplateRefs[tierName]["spaceRoles"].(map[string]string) {
					assert.Equal(t, tmpl, actualSpaceRoleTmplRefs[role])
				}
			}
		})

		t.Run("create then update with new tier templates", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.Must(uuid.NewV4()).String()[:7]
			clt := testsupport.NewFakeClient(t)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testassets)
			require.NoError(t, err)

			// given a new set of tier templates (same content but new revisions, which is what we'll want to check here)
			testassets = assets.NewAssets(testnstemplatetiers.AssetNames, func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte(
						`advanced/based_on_tier: "111111a"` + "\n" +
							`base/cluster: "222222a"` + "\n" +
							`base/ns_dev: "222222b"` + "\n" +
							`base/ns_stage: "222222c"` + "\n" +
							`base/spacerole_admin: "222222d"` + "\n" +
							`nocluster/ns_dev: "333333a"` + "\n" +
							`nocluster/ns_stage: "333333b"` + "\n" +
							`nocluster/spacerole_admin: "333333c"` + "\n" +
							`appstudio/cluster: "444444a"` + "\n" +
							`appstudio/ns_appstudio: "444444b"` + "\n" +
							`appstudio/spacerole_admin: "444444c"` + "\n" +
							`appstudio/spacerole_viewer: "444444d"` + "\n"), nil
				}
				// return default content for other assets
				return testnstemplatetiers.Asset(name)
			})

			// when calling CreateOrUpdateResources a second time
			err = nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testassets)

			// then
			require.NoError(t, err)
			// verify that all TierTemplate CRs for the new revisions were created
			tierTmpls := toolchainv1alpha1.TierTemplateList{}
			err = clt.List(context.TODO(), &tierTmpls, client.InNamespace(namespace))
			require.NoError(t, err)
			require.Len(t, tierTmpls.Items, 30) // two versions of: 4 items for advanced and base tiers + 3 for nocluster tier + 4 for appstudio
			for _, tierTmpl := range tierTmpls.Items {
				assert.Equal(t, int64(1), tierTmpl.ObjectMeta.Generation) // unchanged
			}

			expectedTemplateRefs := map[string]map[string]interface{}{
				"advanced": {
					"clusterresources": "advanced-clusterresources-111111a-222222a",
					"namespaces": []string{
						"advanced-dev-111111a-222222b",
						"advanced-stage-111111a-222222c",
					},
					"spaceRoles": map[string]string{
						"admin": "advanced-admin-111111a-222222d",
					},
				},
				"base": {
					"clusterresources": "base-clusterresources-222222a-222222a",
					"namespaces": []string{
						"base-dev-222222b-222222b",
						"base-stage-222222c-222222c",
					},
					"spaceRoles": map[string]string{
						"admin": "base-admin-222222d-222222d",
					},
				},
				"nocluster": {
					"namespaces": []string{
						"nocluster-dev-333333a-333333a",
						"nocluster-stage-333333b-333333b",
					},
					"spaceRoles": map[string]string{
						"admin": "nocluster-admin-333333c-333333c",
					},
				},
				"appstudio": {
					"clusterresources": "appstudio-clusterresources-444444a-444444a",
					"namespaces": []string{
						"appstudio-dev-444444a-444444a",
						"appstudio-stage-444444b-444444b",
					},
					"spaceRoles": map[string]string{
						"admin":  "appstudio-admin-444444c-444444c",
						"viewer": "appstudio-viewer-444444d-444444d",
					},
				},
			}
			// verify that the 3 NStemplateTier CRs were updated
			for _, tierName := range []string{"advanced", "base", "nocluster"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(2), tier.ObjectMeta.Generation)

				// check the `clusteresources` templateRefs
				if tier.Name == "nocluster" {
					assert.Nil(t, tier.Spec.ClusterResources)
				} else {
					require.NotNil(t, tier.Spec.ClusterResources)
					assert.Equal(t, expectedTemplateRefs[tierName]["clusterresources"], tier.Spec.ClusterResources.TemplateRef)
				}

				// check the `namespaces` templateRefs
				actualTemplateRefs := make([]string, len(tier.Spec.Namespaces))
				for i, ns := range tier.Spec.Namespaces {
					actualTemplateRefs[i] = ns.TemplateRef
				}
				assert.ElementsMatch(t, expectedTemplateRefs[tierName]["namespaces"], actualTemplateRefs)

				// check the `spaceRoles` templateRefs
				actualSpaceRoleTmplRefs := make(map[string]string, len(tier.Spec.SpaceRoles))
				for i, r := range tier.Spec.SpaceRoles {
					actualSpaceRoleTmplRefs[i] = r.TemplateRef
				}
				for role, tmpl := range expectedTemplateRefs[tierName]["spaceRoles"].(map[string]string) {
					assert.Equal(t, tmpl, actualSpaceRoleTmplRefs[role])
				}
			}
		})
	})

	t.Run("failures", func(t *testing.T) {

		namespace := "host-operator" + uuid.Must(uuid.NewV4()).String()[:7]

		t.Run("failed to read assets", func(t *testing.T) {
			// given
			fakeAssets := assets.NewAssets(testnstemplatetiers.AssetNames, func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte("advanced-code: abcdef"), nil
				}
				// error occurs when fetching the content of the 'advanced-code.yaml' template
				return nil, errors.Errorf("an error")
			})
			clt := testsupport.NewFakeClient(t)
			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, fakeAssets)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to create NSTemplateTier generator: unable to load templates: an error", err.Error()) // error occurred while creating TierTemplate resources
		})

		t.Run("nstemplatetiers", func(t *testing.T) {

			t.Run("failed to create nstemplatetiers", func(t *testing.T) {
				// given
				clt := testsupport.NewFakeClient(t)
				clt.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "NSTemplateTier" {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Create(ctx, obj, opts...)
				}
				assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
				// when
				err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, assets)
				// then
				require.Error(t, err)
				assert.Regexp(t, "unable to create NSTemplateTiers: unable to create or update the '\\w+' NSTemplateTier: unable to create resource of kind: NSTemplateTier, version: v1alpha1: an error", err.Error())
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
				clt.MockUpdate = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "NSTemplateTier" {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Update(ctx, obj, opts...)
				}
				testassets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
				// when
				err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testassets)
				// then
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unable to create NSTemplateTiers: unable to create or update the 'advanced' NSTemplateTier: unable to create resource of kind: NSTemplateTier, version: v1alpha1: unable to update the resource")
			})
		})

		t.Run("tiertemplates", func(t *testing.T) {

			t.Run("failed to create nstemplatetiers", func(t *testing.T) {
				// given
				clt := testsupport.NewFakeClient(t)
				clt.MockCreate = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.TierTemplate); ok {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Create(ctx, obj, opts...)
				}
				testassets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
				// when
				err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testassets)
				// then
				require.Error(t, err)
				assert.Regexp(t, fmt.Sprintf("unable to create the '\\w+-\\w+-\\w+-\\w+' TierTemplate in namespace '%s'", namespace), err.Error()) // we can't tell for sure which namespace will fail first, but the error should match the given regex
			})
		})
	})
}
