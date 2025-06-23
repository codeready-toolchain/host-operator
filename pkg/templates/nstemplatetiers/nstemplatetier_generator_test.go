package nstemplatetiers_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/deploy"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/constants"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/gofrs/uuid"
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
	"intelmedium",
	"intellarge",
	"intelxlarge",
	"test",
}

func nsTypes(tier string) []string {
	switch tier {
	case "base1ns", "base1nsnoidling", "base1ns6didler", "intelmedium", "intellarge", "intelxlarge", "test":
		return []string{"dev"}
	default:
		return []string{"dev", "stage"}
	}
}

func roles(_ string) []string {
	return []string{"admin"}
}

func TestSyncResourcesWitProdAssets(t *testing.T) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := "host-operator-" + uuid.Must(uuid.NewV4()).String()[:7]
	cl := commontest.NewFakeClient(t)

	// when
	err = nstemplatetiers.SyncResources(context.TODO(), s, cl, namespace)

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
		for _, nsTypeName := range nsTypes(tier) {
			templateName := verifyTierTemplate(t, cl, namespace, tier, nsTypeName)
			expectedNamespaceTmplRefs[tier] = append(expectedNamespaceTmplRefs[tier], templateName)
		}
		for _, role := range roles(tier) {
			roleName := verifyTierTemplate(t, cl, namespace, tier, role)
			if expectedSpaceRoleTmplRefs[tier] == nil {
				expectedSpaceRoleTmplRefs[tier] = map[string]string{}
			}
			expectedSpaceRoleTmplRefs[tier][role] = roleName
		}
		templateName := verifyTierTemplate(t, cl, namespace, tier, "clusterresources")
		expectedClusterResourcesTmplRef[tier] = templateName
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
		t.Run("nstemplatetiers", func(t *testing.T) {
			t.Run("failed to create nstemplatetiers", func(t *testing.T) {
				// given
				clt := commontest.NewFakeClient(t)
				clt.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == "NSTemplateTier" && obj.GetName() == "base" {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Create(ctx, obj, opts...)
				}
				// when
				err := nstemplatetiers.SyncResources(context.TODO(), s, clt, namespace)
				// then
				require.Error(t, err)
				assert.Regexp(t, "unable to create NSTemplateTiers: unable to create or update the 'base' NSTemplateTier: unable to create resource of kind: NSTemplateTier, version: v1alpha1: an error", err.Error())
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
					if obj.GetObjectKind().GroupVersionKind().Kind == "NSTemplateTier" && obj.GetName() == "advanced" {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Update(ctx, obj, opts...)
				}

				// when
				err := nstemplatetiers.SyncResources(context.TODO(), s, clt, namespace)
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
					if strings.HasPrefix(obj.GetName(), "base1ns-dev-") {
						// simulate a client/server error
						return errors.Errorf("an error")
					}
					return clt.Client.Create(ctx, obj, opts...)
				}
				// when
				err := nstemplatetiers.SyncResources(context.TODO(), s, clt, namespace)
				// then
				require.Error(t, err)
				assert.Regexp(t, fmt.Sprintf("unable to create TierTemplates: unable to create the 'base1ns-dev-\\w+-\\w+' TierTemplate in namespace '%s'", namespace), err.Error()) // we can't tell for sure which namespace will fail first, but the error should match the given regex
			})
		})
	})
	t.Run("failed to load assets", func(t *testing.T) {
		// when
		_, _, err := nstemplatetiers.LoadFiles(deploy.NSTemplateTiersFS, "/"+nstemplatetiers.NsTemplateTierRootDir)
		// then
		require.Error(t, err)
		assert.Equal(t, "unable to load templates: open /templates/nstemplatetiers/metadata.yaml: file does not exist", err.Error()) // error occurred while creating TierTemplate resources
	})
	t.Run("bundled tiers", func(t *testing.T) {
		for _, setup := range []struct {
			bundled          bool
			annotatedBundled bool
			used             bool
		}{
			{
				bundled:          false,
				annotatedBundled: false,
				used:             false,
			},
			{
				bundled:          false,
				annotatedBundled: false,
				used:             true,
			},
			{
				bundled:          false,
				annotatedBundled: true,
				used:             false,
			},
			{
				bundled:          false,
				annotatedBundled: true,
				used:             true,
			},
			{
				bundled:          true,
				annotatedBundled: false,
				used:             false,
			},
			{
				bundled:          true,
				annotatedBundled: false,
				used:             true,
			},
			{
				bundled:          true,
				annotatedBundled: true,
				used:             false,
			},
			{
				bundled:          true,
				annotatedBundled: true,
				used:             true,
			},
		} {
			t.Run(fmt.Sprintf("tier: bundled=%v, annotatedBundled=%v, used=%v", setup.bundled, setup.annotatedBundled, setup.used), func(t *testing.T) {
				// given
				testTier := &toolchainv1alpha1.NSTemplateTier{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testTier",
						Namespace: namespace,
						// in production, the finalizer is added by the controller.
						// Here we add it manually to be able to test the annotation removal.
						Finalizers: []string{"dummy"},
					},
				}
				objs := []runtimeclient.Object{testTier}

				if setup.annotatedBundled {
					testTier.Annotations = map[string]string{toolchainv1alpha1.BundledAnnotationKey: constants.BundledWithHostOperatorAnnotationValue}
				}

				if setup.bundled {
					testTier.Name = "base" // use the name of the tier that we know is bundled
				}

				if setup.used {
					space := &toolchainv1alpha1.Space{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-space",
							Namespace: testTier.Namespace,
							Labels:    map[string]string{hash.TemplateTierHashLabelKey(testTier.Name): "1234"},
						},
					}
					objs = append(objs, space)
				}

				clt := commontest.NewFakeClient(t, objs...)

				// when
				err := nstemplatetiers.SyncResources(context.TODO(), clt.Scheme(), clt, namespace)
				inCluster := &toolchainv1alpha1.NSTemplateTier{}
				gerr := clt.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(testTier), inCluster)

				// then
				shouldBeDeleted := !setup.bundled && setup.annotatedBundled && !setup.used
				shouldBeAnnotated := setup.bundled || setup.annotatedBundled
				require.NoError(t, err)
				require.NoError(t, gerr)
				if shouldBeDeleted {
					assert.NotNil(t, inCluster.DeletionTimestamp)
				}
				if shouldBeAnnotated {
					assert.Equal(t, constants.BundledWithHostOperatorAnnotationValue, inCluster.Annotations[toolchainv1alpha1.BundledAnnotationKey])
				} else {
					assert.Empty(t, inCluster.Annotations)
				}
			})
		}
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
