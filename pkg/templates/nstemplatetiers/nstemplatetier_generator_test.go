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

	testassets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)

	t.Run("ok", func(t *testing.T) {

		expectedTemplateRefs := map[string]map[string][]string{
			"advanced": {
				"clusterresources": {"advanced-clusterresources-654321a"},
				"namespaces": {
					"advanced-code-123456a",
					"advanced-dev-123456b",
					"advanced-stage-123456c",
				},
			},
			"basic": {
				"clusterresources": {"basic-clusterresources-654321b"},
				"namespaces": {
					"basic-code-123456d",
					"basic-dev-123456e",
					"basic-stage-123456f",
				},
			},
			"team": {
				"clusterresources": {"team-clusterresources-654321c"},
				"namespaces": {
					"team-dev-123456g",
					"team-stage-123456h",
				},
			},
			"nocluster": {
				"namespaces": {
					"nocluster-code-123456i",
					"nocluster-dev-123456j",
					"nocluster-stage-1234567",
				},
			},
		}

		t.Run("create only", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
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

			// verify that 18 TierTemplates were created, 14 TierTemplates and 4 templates for NSTemplateTier (see `testnstemplatetiers.AssetNames`)
			tierTmpls = toolchainv1alpha1.TierTemplateList{}
			err = clt.List(context.TODO(), &tierTmpls, client.InNamespace(namespace))
			require.NoError(t, err)
			require.Len(t, tierTmpls.Items, len(testnstemplatetiers.AssetNames())-5) // exclude `metadata.yml` and `tier.yaml` from the AssetNames, they do not result in a TemplateTier resource
			names := make([]string, len(testnstemplatetiers.AssetNames())-5)
			for i, tierTmpl := range tierTmpls.Items {
				names[i] = tierTmpl.Name
			}
			fmt.Printf("names: %v\n", names)
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
					assert.Nil(t, tier.Spec.ClusterResources) // "nocluster" tier should not have cluster resources set
				} else {
					require.NotNil(t, tier.Spec.ClusterResources)
					assert.Equal(t, expectedTemplateRefs[tierName]["clusterresources"][0], tier.Spec.ClusterResources.TemplateRef)
				}
				// retain the actual TemplateRefs
				actualTemplateRefs := make([]string, len(tier.Spec.Namespaces))
				for i, ns := range tier.Spec.Namespaces {
					actualTemplateRefs[i] = ns.TemplateRef
				}
				// now check against the expected TemplateRefs
				assert.ElementsMatch(t, expectedTemplateRefs[tierName]["namespaces"], actualTemplateRefs)
			}
		})

		t.Run("create then update with same tier templates", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
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
			require.Len(t, tierTmpls.Items, len(testnstemplatetiers.AssetNames())-5) // exclude `metadata.yml` and `tier.yaml` from the AssetNames, it does not result in a TemplateTier resource
			for _, tierTmpl := range tierTmpls.Items {
				assert.Equal(t, int64(1), tierTmpl.ObjectMeta.Generation) // unchanged
			}

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
					assert.Equal(t, expectedTemplateRefs[tierName]["clusterresources"][0], tier.Spec.ClusterResources.TemplateRef)
				}
				// retain the actual TemplateRefs
				actualTemplateRefs := make([]string, len(tier.Spec.Namespaces))
				for i, ns := range tier.Spec.Namespaces {
					actualTemplateRefs[i] = ns.TemplateRef
				}
				// now check against the expected TemplateRefs
				assert.ElementsMatch(t, expectedTemplateRefs[tierName]["namespaces"], actualTemplateRefs)
			}
		})

		t.Run("create then update with new tier templates", func(t *testing.T) {
			// given
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			clt := testsupport.NewFakeClient(t)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testassets)
			require.NoError(t, err)

			// given a new set of tier templates (same content but new revisions, which is what we'll want to check here)
			testassets = assets.NewAssets(testnstemplatetiers.AssetNames, func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte(`advanced/cluster: "111111a"` + "\n" +
						`advanced/ns_code: "222222a"` + "\n" +
						`advanced/ns_dev: "222222b"` + "\n" +
						`advanced/ns_stage: "222222c"` + "\n" +
						`basic/cluster: "111111b"` + "\n" +
						`basic/ns_code: "222222d"` + "\n" +
						`basic/ns_dev: "222222e"` + "\n" +
						`basic/ns_stage: "222222f"` + "\n" +
						`team/cluster: "111111c"` + "\n" +
						`team/ns_dev: "222222g"` + "\n" +
						`team/ns_stage: "222222h"` + "\n" +
						`nocluster/ns_code: "222222i"` + "\n" +
						`nocluster/ns_dev: "222222j"` + "\n" +
						`nocluster/ns_stage: "2222227"`), nil
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
			require.Len(t, tierTmpls.Items, 2*(len(testnstemplatetiers.AssetNames())-5)) // 2 sets of TierTemplates, but exclude the `metadata.yml`s and `tier.yaml`s from the AssetNames, they don't result in a TemplateTier resource
			for _, tierTmpl := range tierTmpls.Items {
				assert.Equal(t, int64(1), tierTmpl.ObjectMeta.Generation) // unchanged
			}

			expectedTemplateRefs := map[string]map[string][]string{
				"advanced": {
					"clusterresources": {"advanced-clusterresources-111111a"},
					"namespaces": {
						"advanced-code-222222a",
						"advanced-dev-222222b",
						"advanced-stage-222222c",
					},
				},
				"basic": {
					"clusterresources": {"basic-clusterresources-111111b"},
					"namespaces": {
						"basic-code-222222d",
						"basic-dev-222222e",
						"basic-stage-222222f",
					},
				},
				"team": {
					"clusterresources": {"team-clusterresources-111111c"},
					"namespaces": {
						"team-dev-222222g",
						"team-stage-222222h",
					},
				},
				"nocluster": {
					"namespaces": {
						"nocluster-code-222222i",
						"nocluster-dev-222222j",
						"nocluster-stage-2222227",
					},
				},
			}
			// verify that the 4 NStemplateTier CRs were updated
			for _, tierName := range []string{"advanced", "basic", "team", "nocluster"} {
				tier := toolchainv1alpha1.NSTemplateTier{}
				err = clt.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, &tier)
				require.NoError(t, err)
				assert.Equal(t, int64(2), tier.ObjectMeta.Generation)
				if tier.Name == "nocluster" {
					assert.Nil(t, tier.Spec.ClusterResources) // "team" tier should not have cluster resources set
				} else {
					require.NotNil(t, tier.Spec.ClusterResources)
					assert.Equal(t, expectedTemplateRefs[tierName]["clusterresources"][0], tier.Spec.ClusterResources.TemplateRef)
				}
				// retain the actual TemplateRefs
				actualTemplateRefs := make([]string, len(tier.Spec.Namespaces))
				for i, ns := range tier.Spec.Namespaces {
					actualTemplateRefs[i] = ns.TemplateRef
				}
				// now check against the expected TemplateRefs
				assert.ElementsMatch(t, expectedTemplateRefs[tierName]["namespaces"], actualTemplateRefs)
			}
		})
	})

	t.Run("failures", func(t *testing.T) {

		namespace := "host-operator" + uuid.NewV4().String()[:7]

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
			assert.Equal(t, "unable to create TierTemplates: unable to load templates: an error", err.Error()) // error occurred while creating TierTemplate resources
		})

		t.Run("nstemplatetiers", func(t *testing.T) {

			t.Run("failed to create nstemplatetiers", func(t *testing.T) {
				// given
				clt := testsupport.NewFakeClient(t)
				clt.MockCreate = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
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
				assert.Regexp(t, "unable to create NSTemplateTiers: unable to create the '\\w+' NSTemplateTier: unable to create resource of kind: NSTemplateTier, version: v1alpha1: unable to create resource of kind: NSTemplateTier, version: v1alpha1: an error", err.Error())
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
				assert.Contains(t, err.Error(), "unable to create NSTemplateTiers: unable to create the 'advanced' NSTemplateTier: unable to create resource of kind: NSTemplateTier, version: v1alpha1: unable to create resource of kind: NSTemplateTier, version: v1alpha1: unable to update the resource")
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
				testassets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
				// when
				err := nstemplatetiers.CreateOrUpdateResources(s, clt, namespace, testassets)
				// then
				require.Error(t, err)
				assert.Regexp(t, fmt.Sprintf("unable to create the '\\w+-\\w+-\\w+' TierTemplate in namespace '%s'", namespace), err.Error()) // we can't tell for sure which namespace will fail first, but the error should match the given regex
			})
		})
	})
}
