package nstemplatetiers_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gofrs/uuid"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var expectedProdTiers = []string{
	"advanced",
	"base",
	"base1ns",
	"base1nsnoidling",
	"base1ns6didler",
	"baselarge",
	"baseextendedidling",
	"test",
	"appstudio",
	"appstudiolarge",
	"appstudio-env",
}

func nsTypes(tier string) []string {
	switch tier {
	case "appstudio", "appstudiolarge":
		return []string{"tenant"}
	case "appstudio-env":
		return []string{"env"}
	case "base1ns", "base1nsnoidling", "base1ns6didler", "test":
		return []string{"dev"}
	default:
		return []string{"dev", "stage"}
	}
}

func roles(tier string) []string {
	switch tier {
	case "appstudio", "appstudio-env", "appstudiolarge":
		return []string{"admin", "maintainer", "contributor"}
	default:
		return []string{"admin"}
	}
}

func TestCreateOrUpdateResourcesWitProdAssets(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := "host-operator-" + uuid.Must(uuid.NewV4()).String()[:7]
	cl := commontest.NewFakeClient(t)
	templateAssets := assets.NewAssets(nstemplatetiers.AssetNames, nstemplatetiers.Asset)

	// when
	err = nstemplatetiers.CreateOrUpdateResources(context.TODO(), s, cl, namespace, templateAssets)

	// then
	require.NoError(t, err)
	nstemplateTiers := &toolchainv1alpha1.NSTemplateTierList{}
	err = cl.List(context.TODO(), nstemplateTiers, runtimeclient.InNamespace(namespace))
	require.NoError(t, err)
	expectedClusterResourcesTmplRef := map[string]string{}
	expectedNamespaceTmplRefs := map[string][]string{}
	expectedSpaceRoleTmplRefs := map[string]map[string]string{}
	require.Len(t, nstemplateTiers.Items, len(expectedProdTiers))
	for _, tier := range expectedProdTiers {
		t.Run(tier, func(t *testing.T) {
			for _, nsTypeName := range nsTypes(tier) {
				t.Run("ns-"+nsTypeName, func(t *testing.T) {
					templateName := verifyTierTemplate(t, cl, namespace, tier, nsTypeName)
					expectedNamespaceTmplRefs[tier] = append(expectedNamespaceTmplRefs[tier], templateName)
				})
			}
			for _, role := range roles(tier) {
				t.Run("spacerole-"+role, func(t *testing.T) {
					roleName := verifyTierTemplate(t, cl, namespace, tier, role)
					if expectedSpaceRoleTmplRefs[tier] == nil {
						expectedSpaceRoleTmplRefs[tier] = map[string]string{}
					}
					expectedSpaceRoleTmplRefs[tier][role] = roleName
				})
			}
			t.Run("clusterresources", func(t *testing.T) {
				templateName := verifyTierTemplate(t, cl, namespace, tier, "clusterresources")
				expectedClusterResourcesTmplRef[tier] = templateName
			})
		})
	}
	// verify that each NSTemplateTier has the ClusterResources, Namespaces and SpaceRoles `TemplateRef` set as expected

	for _, nstmplTier := range nstemplateTiers.Items {
		// verify tier configuration

		require.Contains(t, expectedProdTiers, nstmplTier.Name)

		require.NotNil(t, nstmplTier.Spec.ClusterResources)
		assert.Equal(t, expectedClusterResourcesTmplRef[nstmplTier.Name], nstmplTier.Spec.ClusterResources.TemplateRef)
		actualNamespaceTmplRefs := []string{}
		for _, ns := range nstmplTier.Spec.Namespaces {
			actualNamespaceTmplRefs = append(actualNamespaceTmplRefs, ns.TemplateRef)
		}
		assert.ElementsMatch(t, expectedNamespaceTmplRefs[nstmplTier.Name], actualNamespaceTmplRefs)

		require.Len(t, nstmplTier.Spec.SpaceRoles, len(expectedSpaceRoleTmplRefs[nstmplTier.Name]))
		for role, templateRef := range expectedSpaceRoleTmplRefs[nstmplTier.Name] {
			assert.Equal(t, nstmplTier.Spec.SpaceRoles[role].TemplateRef, templateRef)
		}

	}

	t.Run("failures", func(t *testing.T) {

		namespace := "host-operator" + uuid.Must(uuid.NewV4()).String()[:7]

		t.Run("failed to read assets", func(t *testing.T) {
			// given
			fakeAssets := assets.NewAssets(nstemplatetiers.AssetNames, func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte("advanced-code: abcdef"), nil
				}
				// error occurs when fetching the content of the 'advanced-code.yaml' template
				return nil, errors.Errorf("an error")
			})
			clt := commontest.NewFakeClient(t)
			// when
			err := nstemplatetiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, fakeAssets)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to load templates: an error", err.Error()) // error occurred while creating TierTemplate resources
		})

		t.Run("nstemplatetiers", func(t *testing.T) {

			t.Run("failed to create nstemplatetiers", func(t *testing.T) {
				// given
				clt := commontest.NewFakeClient(t)
				clt.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "NSTemplateTier" {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Create(ctx, obj, opts...)
				}
				assets := assets.NewAssets(nstemplatetiers.AssetNames, nstemplatetiers.Asset)
				// when
				err := nstemplatetiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, assets)
				// then
				require.Error(t, err)
				assert.Regexp(t, "unable to create or update the '\\w+' NSTemplateTier: unable to create resource of kind: NSTemplateTier, version: v1alpha1: an error", err.Error())
			})

			t.Run("failed to update nstemplatetiers", func(t *testing.T) {
				// given
				// initialize the client with an existing `advanced` NSTemplatetier
				clt := commontest.NewFakeClient(t, &toolchainv1alpha1.NSTemplateTier{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "advanced",
					},
				})
				clt.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "NSTemplateTier" {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Update(ctx, obj, opts...)
				}
				testassets := assets.NewAssets(nstemplatetiers.AssetNames, nstemplatetiers.Asset)
				// when
				err := nstemplatetiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, testassets)
				// then
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unable to create NSTemplateTiers: unable to create or update the 'advanced' NSTemplateTier: unable to create resource of kind: NSTemplateTier, version: v1alpha1: unable to update the resource")
			})
		})

		t.Run("tiertemplates", func(t *testing.T) {

			t.Run("failed to create nstemplatetiers", func(t *testing.T) {
				// given
				clt := commontest.NewFakeClient(t)
				clt.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
					if _, ok := obj.(*toolchainv1alpha1.TierTemplate); ok {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Create(ctx, obj, opts...)
				}
				testassets := assets.NewAssets(nstemplatetiers.AssetNames, nstemplatetiers.Asset)
				// when
				err := nstemplatetiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, testassets)
				// then
				require.Error(t, err)
				assert.Regexp(t, fmt.Sprintf("unable to create the '\\w+-\\w+-\\w+-\\w+' TierTemplate in namespace '%s'", namespace), err.Error()) // we can't tell for sure which namespace will fail first, but the error should match the given regex
			})
		})

		t.Run("missing asset", func(t *testing.T) {
			// given
			fakeAssets := func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return nstemplatetiers.Asset(name)
				}
				return nil, fmt.Errorf("an error occurred")
			}
			tmplAssets := assets.NewAssets(nstemplatetiers.AssetNames, fakeAssets)
			clt := commontest.NewFakeClient(t)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, tmplAssets)

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load templates: an error occurred")
		})

		t.Run("missing metadata", func(t *testing.T) {
			// given
			fakeAssets := func(name string) ([]byte, error) {
				return nil, fmt.Errorf("an error occurred")
			}
			tmplAssets := assets.NewAssets(nstemplatetiers.AssetNames, fakeAssets)
			clt := commontest.NewFakeClient(t)

			// when
			err := nstemplatetiers.CreateOrUpdateResources(context.TODO(), s, clt, namespace, tmplAssets)

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load templates: an error occurred")
		})
	})
}

func verifyTierTemplate(t *testing.T, cl *commontest.FakeClient, namespace, tierName, typeName string) string {
	tierTemplates := &toolchainv1alpha1.TierTemplateList{}
	require.NoError(t, cl.List(context.TODO(), tierTemplates, runtimeclient.InNamespace(namespace)))

	for _, template := range tierTemplates.Items {
		if template.Spec.TierName == tierName && template.Spec.Type == typeName {
			splitName := strings.Split(template.Name[len(tierName)+1:], "-")
			require.Len(t, splitName, 3)
			assert.Equal(t, tierName, template.Name[:len(tierName)])
			assert.Equal(t, typeName, splitName[0])
			assert.Equal(t, fmt.Sprintf("%s-%s", splitName[1], splitName[2]), template.Spec.Revision)
			assert.Equal(t, tierName, template.Spec.TierName)
			assert.Equal(t, typeName, template.Spec.Type)
			assert.NotEmpty(t, template.Spec.Template)
			assert.NotEmpty(t, template.Spec.Template.Name)
			return template.Name
		}
	}
	require.Fail(t, fmt.Sprintf("the TierTemplate for NSTemplateTier '%s' and of the type '%s' wasn't found", tierName, typeName))
	return ""
}
