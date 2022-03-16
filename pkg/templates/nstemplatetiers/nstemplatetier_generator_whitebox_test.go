package nstemplatetiers

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	texttemplate "text/template"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/ghodss/yaml"
	"github.com/gofrs/uuid"
	templatev1 "github.com/openshift/api/template/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var expectedProdTiers = map[string]bool{
	"advanced":                 true, // tier_name: true/false (if based on the other tier)
	"base":                     false,
	"baselarge":                true,
	"baseextended":             true,
	"baseextendedidling":       true,
	"basedeactivationdisabled": true,
	"hackathon":                true,
	"test":                     false,
	"appstudio":                false,
}

var expectedTestTiers = map[string]bool{
	"advanced":  true, // tier_name: true/false (if based on the other tier)
	"base":      false,
	"nocluster": false,
	"appstudio": false,
}

func nsTypes(tier string) []string {
	switch tier {
	case "appstudio":
		return []string{"appstudio"}
	default:
		return []string{"dev", "stage"}
	}
}

func roles(tier string) []string {
	switch tier {
	case "appstudio":
		return []string{"admin", "viewer"}
	default:
		return []string{"admin"}
	}
}

func isNamespaceType(expectedTiers map[string]bool, typeName string) bool {
	for _, tier := range tiers(expectedTiers) {
		for _, t := range nsTypes(tier) {
			if t == typeName {
				return true
			}
		}
	}
	return false
}

func isSpaceRole(expectedTiers map[string]bool, roleName string) bool {
	for _, tier := range tiers(expectedTiers) {
		for _, r := range roles(tier) {
			if r == roleName {
				return true
			}
		}
	}
	return false
}

func tiers(expectedTiers map[string]bool) []string {
	tt := make([]string, 0, len(expectedTiers))
	for tier := range expectedTiers {
		tt = append(tt, tier)
	}
	return tt
}

func assertKnownTier(t *testing.T, expectedTiers map[string]bool, tier string) {
	assert.Contains(t, expectedTiers, tier, "encountered an unexpected tier: %s", tier)
}

func basedOnOtherTier(expectedTiers map[string]bool, tier string) bool {
	return expectedTiers[tier]
}

func TestLoadTemplatesByTiers(t *testing.T) {

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			assets := assets.NewAssets(AssetNames, Asset)
			// when
			tmpls, err := loadTemplatesByTiers(assets)
			// then
			require.NoError(t, err)
			require.Len(t, tmpls, 9)
			require.NotContains(t, "foo", tmpls) // make sure that the `foo: bar` entry was ignored
			for _, tier := range tiers(expectedProdTiers) {
				t.Run(tier, func(t *testing.T) {
					for _, nsTypeName := range nsTypes(tier) {
						t.Run("ns-"+nsTypeName, func(t *testing.T) {
							if basedOnOtherTier(expectedProdTiers, tier) {
								assert.Empty(t, tmpls[tier].rawTemplates.namespaceTemplates[nsTypeName].revision)
								assert.Empty(t, tmpls[tier].rawTemplates.namespaceTemplates[nsTypeName].content)
							} else {
								assert.NotEmpty(t, tmpls[tier].rawTemplates.namespaceTemplates[nsTypeName].revision)
								assert.NotEmpty(t, tmpls[tier].rawTemplates.namespaceTemplates[nsTypeName].content)
							}
						})
					}
					for _, role := range roles(tier) {
						t.Run("spacerole-"+role, func(t *testing.T) {
							if basedOnOtherTier(expectedProdTiers, tier) {
								assert.Empty(t, tmpls[tier].rawTemplates.spaceroleTemplates[role].revision)
								assert.Empty(t, tmpls[tier].rawTemplates.spaceroleTemplates[role].content)
							} else {
								assert.NotEmpty(t, tmpls[tier].rawTemplates.spaceroleTemplates[role].revision)
								assert.NotEmpty(t, tmpls[tier].rawTemplates.spaceroleTemplates[role].content)
							}
						})
					}
					t.Run("clusterresources", func(t *testing.T) {
						if basedOnOtherTier(expectedProdTiers, tier) {
							assert.Nil(t, tmpls[tier].rawTemplates.clusterTemplate)
						} else {
							require.NotNil(t, tmpls[tier].rawTemplates.clusterTemplate)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.clusterTemplate.revision)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.clusterTemplate.content)
						}
					})
					t.Run("based_on_tier", func(t *testing.T) {
						if basedOnOtherTier(expectedProdTiers, tier) {
							require.NotNil(t, tmpls[tier].basedOnTier)
						} else {
							require.Nil(t, tmpls[tier].basedOnTier)
						}
					})
				})
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
			// when
			tmpls, err := loadTemplatesByTiers(assets)
			// then
			require.NoError(t, err)
			require.Len(t, tmpls, 4)
			require.NotContains(t, "foo", tmpls) // make sure that the `foo: bar` entry was ignored

			for _, tier := range tiers(expectedTestTiers) {
				t.Run(tier, func(t *testing.T) {
					for _, typeName := range nsTypes(tier) {
						t.Run("ns-"+typeName, func(t *testing.T) {
							require.NotNil(t, tmpls[tier])
							if basedOnOtherTier(expectedTestTiers, tier) {
								assert.Empty(t, tmpls[tier].rawTemplates.namespaceTemplates[typeName].revision)
								assert.Empty(t, tmpls[tier].rawTemplates.namespaceTemplates[typeName].content)
							} else {
								assert.NotEmpty(t, tmpls[tier].rawTemplates.namespaceTemplates[typeName].revision)
								assert.NotEmpty(t, tmpls[tier].rawTemplates.namespaceTemplates[typeName].content)
							}
						})
					}
					for _, role := range roles(tier) {
						t.Run("spacerole-"+role, func(t *testing.T) {
							if basedOnOtherTier(expectedProdTiers, tier) {
								assert.Empty(t, tmpls[tier].rawTemplates.spaceroleTemplates[role].revision)
								assert.Empty(t, tmpls[tier].rawTemplates.spaceroleTemplates[role].content)
							} else {
								assert.NotEmpty(t, tmpls[tier].rawTemplates.spaceroleTemplates[role].revision)
								assert.NotEmpty(t, tmpls[tier].rawTemplates.spaceroleTemplates[role].content)
							}
						})
					}
					t.Run("cluster", func(t *testing.T) {
						require.NotNil(t, tmpls[tier].rawTemplates)
						if basedOnOtherTier(expectedTestTiers, tier) {
							assert.Nil(t, tmpls[tier].rawTemplates.clusterTemplate)
						} else if tier != "nocluster" {
							require.NotNil(t, tmpls[tier].rawTemplates.clusterTemplate)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.clusterTemplate.revision)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.clusterTemplate.content)
						} else {
							require.Nil(t, tmpls[tier].rawTemplates.clusterTemplate)
						}
					})
					t.Run("based_on_tier", func(t *testing.T) {
						if basedOnOtherTier(expectedTestTiers, tier) {
							require.NotNil(t, tmpls[tier].basedOnTier)
						} else {
							require.Nil(t, tmpls[tier].basedOnTier)
						}
					})
				})
			}
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unparseable content", func(t *testing.T) {
			// given
			fakeAssets := func(name string) ([]byte, error) {
				return []byte("foo::bar"), nil
			}
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, fakeAssets)
			// when
			_, err := loadTemplatesByTiers(assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load templates: yaml: unmarshal errors:")
		})

		t.Run("missing metadata", func(t *testing.T) {
			// given
			fakeAssets := func(name string) ([]byte, error) {
				return nil, fmt.Errorf("an error occurred")
			}
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, fakeAssets)
			// when
			_, err := loadTemplatesByTiers(assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load templates: an error occurred")
		})

		t.Run("missing asset", func(t *testing.T) {
			// given
			fakeAssets := func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return testnstemplatetiers.Asset(name)
				}
				return nil, fmt.Errorf("an error occurred")
			}
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, fakeAssets)
			// when
			_, err := loadTemplatesByTiers(assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load templates: an error occurred")
		})

		t.Run("invalid name format", func(t *testing.T) {
			// given
			fakeAssetNames := func() []string {
				return []string{`.DS_Store`, `metadata.yaml`} // '/advanced/foo.yaml' is not a valid filename
			}
			fakeAssets := func(name string) ([]byte, error) {
				switch name {
				case "metadata.yaml":
					return []byte(`advanced/foo.yaml: "123456a"`), nil // just make sure the asset exists
				case ".DS_Store":
					return []byte(`foo:bar`), nil // just make sure the asset exists
				default:
					return testnstemplatetiers.Asset(name)
				}
			}
			assets := assets.NewAssets(fakeAssetNames, fakeAssets)
			// when
			_, err := loadTemplatesByTiers(assets)
			// then
			require.Error(t, err)
			assert.EqualError(t, err, "unable to load templates: invalid name format for file '.DS_Store'")
		})

		t.Run("invalid filename scope", func(t *testing.T) {
			// given
			fakeAssetNames := func() []string {
				return []string{`metadata.yaml`, `advanced/foo.yaml`} // '/advanced/foo.yaml' is not a valid filename
			}
			fakeAssets := func(name string) ([]byte, error) {
				switch name {
				case "metadata.yaml":
					return []byte(`advanced/foo.yaml: "123456a"`), nil // just make sure the asset exists
				case "advanced/foo.yaml":
					return []byte(`foo:bar`), nil // just make sure the asset exists
				default:
					return testnstemplatetiers.Asset(name)
				}
			}
			assets := assets.NewAssets(fakeAssetNames, fakeAssets)
			// when
			_, err := loadTemplatesByTiers(assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load templates: unknown scope for file 'advanced/foo.yaml'")
		})

		t.Run("should fail when tier contains a mix of based_on_tier.yaml file together with a regular template file", func(t *testing.T) {
			// given
			s := scheme.Scheme
			err := apis.AddToScheme(s)
			require.NoError(t, err)
			clt := test.NewFakeClient(t)

			for _, tmplName := range []string{"cluster.yaml", "ns_dev.yaml", "ns_stage.yaml", "tier.yaml"} {
				t.Run("for template name "+tmplName, func(t *testing.T) {
					// given
					testassets := assets.NewAssets(func() []string {
						return append(testnstemplatetiers.AssetNames(), fmt.Sprintf("advanced/%s", tmplName))

					}, func(path string) (bytes []byte, e error) {
						if path == "advanced/"+tmplName {
							return []byte(""), nil
						}
						return testnstemplatetiers.Asset(path)
					})

					// when
					_, err := newTierGenerator(s, clt, test.HostOperatorNs, testassets)

					// then
					require.EqualError(t, err, "the tier advanced contains a mix of based_on_tier.yaml file together with a regular template file")
				})
			}
		})
	})
}

func TestNewNSTemplateTier(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			namespace := "host-operator-" + uuid.Must(uuid.NewV4()).String()[:7]
			assets := assets.NewAssets(AssetNames, Asset)

			// when
			// uses the `Asset` funcs generated in the `pkg/templates/nstemplatetiers/` subpackages
			tc, err := newTierGenerator(s, nil, namespace, assets)
			require.NoError(t, err)
			// then
			require.NotEmpty(t, tc.templatesByTier)
			// verify that each NSTemplateTier has the ClusterResources, Namespaces and SpaceRoles `TemplateRef` set as expected
			for _, tierData := range tc.templatesByTier {

				for _, nstmplTierObj := range tierData.objects {
					// verify tier configuration
					nstmplTier := runtimeObjectToNSTemplateTier(t, s, nstmplTierObj)
					require.NotNil(t, nstmplTier)
					assert.Equal(t, namespace, nstmplTier.Namespace)

					assertKnownTier(t, expectedProdTiers, nstmplTier.Name)

					// verify tier templates
					var expectedClusterResourcesTmplRef string
					expectedNamespaceTmplRefs := []string{}
					expectedSpaceRoleTmplRefs := []string{}
					for _, tierTmpl := range tierData.tierTemplates {
						switch {
						case strings.Contains(tierTmpl.Name, "clusterresources"):
							expectedClusterResourcesTmplRef = tierTmpl.Name
						case strings.Contains(tierTmpl.Name, "admin") || strings.Contains(tierTmpl.Name, "viewer"):
							expectedSpaceRoleTmplRefs = append(expectedSpaceRoleTmplRefs, tierTmpl.Name)
						default:
							expectedNamespaceTmplRefs = append(expectedNamespaceTmplRefs, tierTmpl.Name)
						}
					}
					require.NotNil(t, nstmplTier.Spec.ClusterResources)
					assert.Equal(t, expectedClusterResourcesTmplRef, nstmplTier.Spec.ClusterResources.TemplateRef)
					actualNamespaceTmplRefs := []string{}
					for _, ns := range nstmplTier.Spec.Namespaces {
						actualNamespaceTmplRefs = append(actualNamespaceTmplRefs, ns.TemplateRef)
					}
					assert.ElementsMatch(t, expectedNamespaceTmplRefs, actualNamespaceTmplRefs)
					actualSpaceRoleTmplRefs := []string{}
					for _, role := range nstmplTier.Spec.SpaceRoles {
						actualSpaceRoleTmplRefs = append(actualSpaceRoleTmplRefs, role.TemplateRef)
					}
					assert.ElementsMatch(t, expectedSpaceRoleTmplRefs, actualSpaceRoleTmplRefs)

				}
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			namespace := "host-operator-" + uuid.Must(uuid.NewV4()).String()[:7]
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
			tc, err := newTierGenerator(s, nil, namespace, assets)
			require.NoError(t, err)
			clusterResourcesRevisions := map[string]string{
				"advanced":  "abcd123-654321a",
				"base":      "654321a-654321a",
				"appstudio": "654321a-654321a",
			}
			namespaceRevisions := map[string]map[string]string{
				"advanced": {
					"dev":   "abcd123-123456b",
					"stage": "abcd123-123456c",
				},
				"base": {
					"dev":   "123456b-123456b",
					"stage": "123456c-123456c",
				},
				"nocluster": {
					"dev":   "123456j-123456j",
					"stage": "1234567-1234567",
				},
				"appstudio": {
					"appstudio": "123456b-123456b",
				},
			}
			spaceRoleRevisions := map[string]map[string]string{
				"advanced": {
					"admin": "abcd123-123456d",
				},
				"base": {
					"admin": "123456d-123456d",
				},
				"nocluster": {
					"admin": "123456k-123456k",
				},
				"appstudio": {
					"admin":  "123456c-123456c",
					"viewer": "123456d-123456d",
				},
			}
			for tier := range namespaceRevisions {
				t.Run(tier, func(t *testing.T) {
					// given
					objects := tc.templatesByTier[tier].objects
					require.Len(t, objects, 1, "expected only 1 NSTemplateTier toolchain object")
					// when
					actual := runtimeObjectToNSTemplateTier(t, s, objects[0])

					// then
					deactivationTimeout := 30
					if tier == "advanced" {
						deactivationTimeout = 0
					}
					expected, err := newNSTemplateTierFromYAML(s, tier, namespace, deactivationTimeout, clusterResourcesRevisions[tier], namespaceRevisions[tier], spaceRoleRevisions[tier])
					require.NoError(t, err)
					// here we don't compare objects because the generated NSTemplateTier
					// has no specific values for the `TypeMeta`: the `APIVersion: toolchain.dev.openshift.com/v1alpha1`
					// and `Kind: NSTemplateTier` should be set by the client using the registered GVK
					assert.Equal(t, expected.ObjectMeta, actual.ObjectMeta)
					assert.Equal(t, expected.Spec, actual.Spec)
				})
			}
		})
	})
}

func TestNewTierTemplate(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	namespace := "host-operator-" + uuid.Must(uuid.NewV4()).String()[:7]

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			assets := assets.NewAssets(AssetNames, Asset)
			// uses the `Asset` funcs generated in the `pkg/templates/nstemplatetiers/` subpackages
			// when
			tc, err := newTierGenerator(s, nil, namespace, assets)

			// then
			require.NoError(t, err)
			decoder := serializer.NewCodecFactory(s).UniversalDeserializer()

			resourceNameRE, err := regexp.Compile(`[a-z0-9\.-]+`)
			require.NoError(t, err)
			for tier, tmpls := range tc.templatesByTier {
				t.Run(tier, func(t *testing.T) {
					for _, actual := range tmpls.tierTemplates {
						t.Run(actual.Name, func(t *testing.T) {
							assert.Equal(t, namespace, actual.Namespace)
							assert.True(t, resourceNameRE.MatchString(actual.Name)) // verifies that the TierTemplate name complies with the DNS-1123 spec
							assert.NotEmpty(t, actual.Spec.Revision)
							assert.Equal(t, tier, actual.Spec.TierName)
							assert.NotEmpty(t, actual.Spec.Type)
							assert.NotEmpty(t, actual.Spec.Template)
							if actual.Spec.Type == "clusterresources" {
								assertClusterResourcesTemplate(t, decoder, actual.Spec.Template, assets, expectedProdTiers, tier)
							} else if isNamespaceType(expectedProdTiers, actual.Spec.Type) {
								assertNamespaceTemplate(t, decoder, actual.Spec.Template, assets, expectedProdTiers, tier, actual.Spec.Type)
							} else if isSpaceRole(expectedProdTiers, actual.Spec.Type) {
								assertSpaceRoleTemplate(t, decoder, actual.Spec.Template, assets, expectedProdTiers, tier, actual.Spec.Type)
							} else {
								t.Errorf("unexpected type of template: '%s'", actual.Spec.Type)
							}
						})
					}
				})
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
			// when
			tc, err := newTierGenerator(s, nil, namespace, assets)

			// then
			require.NoError(t, err)
			decoder := serializer.NewCodecFactory(s).UniversalDeserializer()

			resourceNameRE, err := regexp.Compile(`[a-z0-9\.-]+`)
			require.NoError(t, err)
			for tier, tmpls := range tc.templatesByTier {
				t.Run(tier, func(t *testing.T) {
					for _, actual := range tmpls.tierTemplates {
						assert.Equal(t, namespace, actual.Namespace)
						assert.True(t, resourceNameRE.MatchString(actual.Name)) // verifies that the TierTemplate name complies with the DNS-1123 spec
						assert.NotEmpty(t, actual.Spec.Revision)
						assert.NotEmpty(t, actual.Spec.TierName)
						assert.NotEmpty(t, actual.Spec.Type)
						assert.NotEmpty(t, actual.Spec.Template)

						if actual.Spec.Type == "clusterresources" {
							assertClusterResourcesTemplate(t, decoder, actual.Spec.Template, assets, expectedTestTiers, tier)
						} else if isNamespaceType(expectedTestTiers, actual.Spec.Type) {
							assertNamespaceTemplate(t, decoder, actual.Spec.Template, assets, expectedTestTiers, tier, actual.Spec.Type)
						} else if isSpaceRole(expectedTestTiers, actual.Spec.Type) {
							assertSpaceRoleTemplate(t, decoder, actual.Spec.Template, assets, expectedTestTiers, tier, actual.Spec.Type)
						} else {
							t.Errorf("unexpected type of template: '%s'", actual.Spec.Type)
						}
					}
				})
			}
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("invalid template", func(t *testing.T) {
			// given
			fakeAssets := assets.NewAssets(testnstemplatetiers.AssetNames, func(name string) ([]byte, error) {
				if name == "metadata.yaml" || strings.HasSuffix(name, "based_on_tier.yaml") {
					return testnstemplatetiers.Asset(name)
				}
				// error occurs when fetching the content of the 'advanced-code.yaml' template
				return []byte("invalid"), nil // return an invalid YAML representation of a Template
			})
			// when
			_, err := newTierGenerator(s, nil, namespace, fakeAssets)

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to generate 'advanced-dev-abcd123-123456b' TierTemplate manifest: couldn't get version/kind; json parse error")
		})
	})
}

func assertClusterResourcesTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, assets assets.Assets, expectedTiers map[string]bool, tier string) {
	if !basedOnOtherTier(expectedTiers, tier) {
		expected := templatev1.Template{}
		content, err := assets.Asset(fmt.Sprintf("%s/cluster.yaml", tier))
		require.NoError(t, err)
		_, _, err = decoder.Decode(content, nil, &expected)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
		assert.NotEmpty(t, actual.Objects)
	}
}

func assertNamespaceTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, assets assets.Assets, expectedTiers map[string]bool, tier, typeName string) {
	var templatePath string
	if basedOnOtherTier(expectedTiers, tier) {
		templatePath = expectedTemplateFromBasedOnTierConfig(t, assets, tier, fmt.Sprintf("ns_%s.yaml", typeName))
	} else {
		templatePath = fmt.Sprintf("%s/ns_%s.yaml", tier, typeName)
	}
	content, err := assets.Asset(templatePath)
	require.NoError(t, err)
	expected := templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.NotEmpty(t, actual.Objects)
}

func assertSpaceRoleTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, assets assets.Assets, expectedTiers map[string]bool, tier, roleName string) {
	var templatePath string
	if basedOnOtherTier(expectedTiers, tier) {
		templatePath = expectedTemplateFromBasedOnTierConfig(t, assets, tier, fmt.Sprintf("spacerole_%s.yaml", roleName))
	} else {
		templatePath = fmt.Sprintf("%s/spacerole_%s.yaml", tier, roleName)
	}
	content, err := assets.Asset(templatePath)
	require.NoError(t, err)
	expected := templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.NotEmpty(t, actual.Objects)
}

func expectedTemplateFromBasedOnTierConfig(t *testing.T, assets assets.Assets, tier, templateFileName string) string {
	basedOnTierContent, err := assets.Asset(fmt.Sprintf("%s/based_on_tier.yaml", tier))
	require.NoError(t, err)
	basedOnTier := BasedOnTier{}
	require.NoError(t, yaml.Unmarshal(basedOnTierContent, &basedOnTier))
	return fmt.Sprintf("%s/%s", basedOnTier.From, templateFileName)
}

func TestNewNSTemplateTiers(t *testing.T) {

	// given
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		// given
		namespace := "host-operator-" + uuid.Must(uuid.NewV4()).String()[:7]
		assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
		// when
		tc, err := newTierGenerator(s, nil, namespace, assets)
		require.NoError(t, err)
		// then
		require.Len(t, tc.templatesByTier, 4)
		for _, name := range []string{"advanced", "base", "nocluster", "appstudio"} {
			tierData, found := tc.templatesByTier[name]
			tierObjs := tierData.objects
			require.Len(t, tierObjs, 1, "expected only 1 NSTemplateTier toolchain object")
			tier := runtimeObjectToNSTemplateTier(t, s, tierObjs[0])

			require.True(t, found)
			assert.Equal(t, name, tier.ObjectMeta.Name)
			assert.Equal(t, namespace, tier.ObjectMeta.Namespace)
			for _, ns := range tier.Spec.Namespaces {
				assert.NotEmpty(t, ns.TemplateRef, "expected namespace reference not empty for tier %v", name)
			}
			if name == "nocluster" {
				assert.Nil(t, tier.Spec.ClusterResources)
			} else {
				require.NotNil(t, tier.Spec.ClusterResources)
				assert.NotEmpty(t, tier.Spec.ClusterResources.TemplateRef)
			}
		}
	})
}

var ExpectedRevisions = map[string]map[string]string{
	"advanced": {
		"based_on_tier": "abcd123",
	},
	"base": {
		"dev":     "123456b",
		"stage":   "123456c",
		"cluster": "654321a",
	},
	"nocluster": {
		"dev":   "123456j",
		"stage": "1234567",
	},
}

// newNSTemplateTierFromYAML generates toolchainv1alpha1.NSTemplateTier using a golang template which is applied to the given tier.
func newNSTemplateTierFromYAML(s *runtime.Scheme, tier, namespace string, deactivationTimeout int, clusterResourcesRevision string, namespaceRevisions map[string]string, spaceRoleRevisions map[string]string) (*toolchainv1alpha1.NSTemplateTier, error) {
	expectedTmpl, err := texttemplate.New("template").Parse(`
{{ $tier := .Tier}}
kind: NSTemplateTier
apiVersion: toolchain.dev.openshift.com/v1alpha1
metadata:
  namespace: {{ .Namespace }}
  name: {{ .Tier }}
spec:
  deactivationTimeoutDays: {{ .DeactivationTimeout }} 
  {{ if .ClusterResourcesRevision }}clusterResources:
    templateRef: {{ .Tier }}-clusterresources-{{ .ClusterResourcesRevision }}
  {{ end }}
  namespaces: 
{{ range $type, $revision := .NamespaceRevisions }}
    - templateRef: {{ $tier }}-{{ $type }}-{{ $revision }}
{{ end }}
  spaceRoles: 
{{ range $role, $revision := .SpaceRoleRevisions }}    
    {{ $role }}:
      templateRef: {{ $tier }}-{{ $role }}-{{ $revision }}
{{ end }}
`)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(nil)
	err = expectedTmpl.Execute(buf, struct {
		Tier                     string
		Namespace                string
		ClusterResourcesRevision string
		NamespaceRevisions       map[string]string
		SpaceRoleRevisions       map[string]string
		DeactivationTimeout      int
	}{
		Tier:                     tier,
		Namespace:                namespace,
		ClusterResourcesRevision: clusterResourcesRevision,
		NamespaceRevisions:       namespaceRevisions,
		SpaceRoleRevisions:       spaceRoleRevisions,
		DeactivationTimeout:      deactivationTimeout,
	})
	if err != nil {
		return nil, err
	}
	result := &toolchainv1alpha1.NSTemplateTier{}
	codecFactory := serializer.NewCodecFactory(s)
	_, _, err = codecFactory.UniversalDeserializer().Decode(buf.Bytes(), nil, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func runtimeObjectToNSTemplateTier(t *testing.T, s *runtime.Scheme, tierObj runtime.Object) *toolchainv1alpha1.NSTemplateTier {
	tier := &toolchainv1alpha1.NSTemplateTier{}
	err := s.Convert(tierObj, tier, nil)
	require.NoError(t, err)
	return tier
}
