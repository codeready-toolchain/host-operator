package nstemplatetiers

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/ghodss/yaml"
	"github.com/gofrs/uuid"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var expectedTiers = map[string]bool{
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

func kinds(tier string) []string {
	switch tier {
	case "appstudio":
		return []string{"appstudio"}
	default:
		return []string{"dev", "stage"}
	}
}

func namespaceKind(kind string) bool {
	for _, k := range allNamespaceKinds() {
		if kind == k {
			return true
		}
	}
	return false
}

func allNamespaceKinds() []string {
	result := make([]string, 0, 3)
	kmap := make(map[string]string)
	for _, tier := range tiers() {
		kk := kinds(tier)
		for _, kind := range kk {
			if _, ok := kmap[kind]; !ok {
				kmap[kind] = kind
				result = append(result, kind)
			}
		}
	}
	return result
}

func tiers() []string {
	tt := make([]string, 0, len(expectedTiers))
	for tier, _ := range expectedTiers {
		tt = append(tt, tier)
	}
	return tt
}

func assertKnownTier(t *testing.T, tier string) {
	assert.Contains(t, expectedTiers, tier, "encountered an unexpected tier: %s", tier)
}

func basedOnOtherTier(tier string) bool {
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
			for _, tier := range tiers() {
				t.Run(tier, func(t *testing.T) {
					for _, kind := range kinds(tier) {
						t.Run(kind, func(t *testing.T) {
							if basedOnOtherTier(tier) {
								assert.Empty(t, tmpls[tier].rawTemplates.namespaceTemplates[kind].revision)
								assert.Empty(t, tmpls[tier].rawTemplates.namespaceTemplates[kind].content)
							} else {
								assert.NotEmpty(t, tmpls[tier].rawTemplates.namespaceTemplates[kind].revision)
								assert.NotEmpty(t, tmpls[tier].rawTemplates.namespaceTemplates[kind].content)
							}
						})
					}
					t.Run("cluster", func(t *testing.T) {
						if basedOnOtherTier(tier) {
							assert.Nil(t, tmpls[tier].rawTemplates.clusterTemplate)
						} else {
							require.NotNil(t, tmpls[tier].rawTemplates.clusterTemplate)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.clusterTemplate.revision)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.clusterTemplate.content)
						}
					})
					t.Run("based_on_tier", func(t *testing.T) {
						if basedOnOtherTier(tier) {
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
			require.Len(t, tmpls, 3)
			require.NotContains(t, "foo", tmpls) // make sure that the `foo: bar` entry was ignored

			for _, tier := range []string{"advanced", "base", "nocluster"} {
				t.Run(tier, func(t *testing.T) {
					for _, kind := range []string{"dev", "stage"} {
						t.Run(kind, func(t *testing.T) {
							if tier != "advanced" {
								assert.Equal(t, ExpectedRevisions[tier][kind], tmpls[tier].rawTemplates.namespaceTemplates[kind].revision)
								assert.NotEmpty(t, tmpls[tier].rawTemplates.namespaceTemplates[kind].content)
							} else {
								assert.Empty(t, tmpls[tier].rawTemplates.namespaceTemplates)
							}
						})
					}
					t.Run("cluster", func(t *testing.T) {
						switch tier {
						case "nocluster", "advanced":
							assert.Nil(t, tmpls[tier].rawTemplates.clusterTemplate)
						default:
							require.NotNil(t, tmpls[tier].rawTemplates.clusterTemplate)
							assert.Equal(t, ExpectedRevisions[tier]["cluster"], tmpls[tier].rawTemplates.clusterTemplate.revision)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.clusterTemplate.content)
						}
					})
					t.Run("based_on_tier", func(t *testing.T) {
						if tier != "advanced" {
							assert.Empty(t, tmpls[tier].rawTemplates.basedOnTier)
						} else {
							assert.Equal(t, ExpectedRevisions[tier]["based_on_tier"], tmpls[tier].rawTemplates.basedOnTier.revision)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.basedOnTier.content)
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
			// verify that each NSTemplateTier has the Namespaces and ClusterResources `TemplateRef` set as expected
			for _, tierData := range tc.templatesByTier {

				for _, nstmplTierObj := range tierData.nstmplTierObjs {
					tierObj := nstmplTierObj

					// verify tier configuration
					nstmplTier := runtimeObjectToNSTemplateTier(t, s, tierObj)
					require.NotNil(t, nstmplTier)
					assert.Equal(t, namespace, nstmplTier.Namespace)

					assertKnownTier(t, nstmplTier.Name)

					// verify tier templates
					require.Len(t, nstmplTier.Spec.Namespaces, len(tierData.tierTemplates)-1) // exclude clusterresources TierTemplate here
					expectedTierTmplNames := make([]string, 0, len(tierData.tierTemplates)-1)
					var clusterResourcesTemplateRef string
					for _, tierTmpl := range tierData.tierTemplates {
						if strings.Contains(tierTmpl.Name, "clusterresources") {
							clusterResourcesTemplateRef = tierTmpl.Name
							continue // skip
						}
						expectedTierTmplNames = append(expectedTierTmplNames, tierTmpl.Name)
					}
					actualTemplateRefs := make([]string, 0, len(nstmplTier.Spec.Namespaces))
					for _, ns := range nstmplTier.Spec.Namespaces {
						actualTemplateRefs = append(actualTemplateRefs, ns.TemplateRef)
					}
					assert.ElementsMatch(t, expectedTierTmplNames, actualTemplateRefs)

					require.NotNil(t, nstmplTier.Spec.ClusterResources)
					assert.Equal(t, clusterResourcesTemplateRef, nstmplTier.Spec.ClusterResources.TemplateRef)
				}
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			namespace := "host-operator-" + uuid.Must(uuid.NewV4()).String()[:7]
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
			tc, err := newTierGenerator(s, nil, namespace, assets)
			require.NoError(t, err)
			namespaceRevisions := map[string]map[string]string{
				"advanced": {
					"dev":   "abcd123-123456b",
					"stage": "abcd123-123456c",
				},
				"base": {
					"dev":   "123456b-123456b",
					"stage": "123456c-123456c",
				},
			}
			clusterResourceQuotaRevisions := map[string]string{
				"advanced": "abcd123-654321a",
				"base":     "654321a-654321a",
			}
			for tier := range namespaceRevisions {
				t.Run(tier, func(t *testing.T) {
					// given
					objects := tc.templatesByTier[tier].nstmplTierObjs
					require.Len(t, objects, 1, "expected only 1 NSTemplateTier toolchain object")
					// when
					actual := runtimeObjectToNSTemplateTier(t, s, objects[0])

					// then
					deactivationTimeout := 30
					if tier == "advanced" {
						deactivationTimeout = 0
					}
					expected, _, err := newNSTemplateTierFromYAML(s, tier, namespace, deactivationTimeout, namespaceRevisions[tier], clusterResourceQuotaRevisions[tier])
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
							if namespaceKind(actual.Spec.Type) {
								assertNamespaceTemplate(t, decoder, actual.Spec.Template, tier, actual.Spec.Type)
							} else if actual.Spec.Type == "clusterresources" {
								assertClusterResourcesTemplate(t, decoder, actual.Spec.Template, tier)
							} else {
								t.Errorf("unexpected kind of template: '%s'", actual.Spec.Type)
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
						switch actual.Spec.Type {
						case "dev", "stage":
							assertTestNamespaceTemplate(t, decoder, actual.Spec.Template, actual.Spec.TierName, actual.Spec.Type)
						case "clusterresources":
							assertTestClusterResourcesTemplate(t, decoder, actual.Spec.Template, actual.Spec.TierName)
						default:
							t.Errorf("unexpected kind of template: '%s'", actual.Spec.Type)
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

func assertClusterResourcesTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, tier string) {
	if !basedOnOtherTier(tier) {
		expected := templatev1.Template{}
		content, err := Asset(fmt.Sprintf("%s/cluster.yaml", tier))
		require.NoError(t, err)
		_, _, err = decoder.Decode(content, nil, &expected)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	}
}

func assertNamespaceTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, tier, kind string) {
	var templatePath string
	if basedOnOtherTier(tier) {
		templatePath = expectedTemplateFromBasedOnTierConfig(t, tier, fmt.Sprintf("ns_%s.yaml", kind))
	} else {
		templatePath = fmt.Sprintf("%s/ns_%s.yaml", tier, kind)
	}
	content, err := Asset(templatePath)
	require.NoError(t, err)
	expected := templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func assertTestClusterResourcesTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, tier string) {
	templatePath := fmt.Sprintf("%s/cluster.yaml", tier)
	if tier == "advanced" {
		templatePath = expectedTemplateFromBasedOnTierConfig(t, tier, "cluster.yaml")
	}
	content, err := testnstemplatetiers.Asset(templatePath)
	require.NoError(t, err)
	expected := templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func assertTestNamespaceTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, tier, kind string) {
	templatePath := fmt.Sprintf("%s/ns_%s.yaml", tier, kind)
	if tier == "advanced" {
		templatePath = expectedTemplateFromBasedOnTierConfig(t, tier, fmt.Sprintf("ns_%s.yaml", kind))
	}
	content, err := testnstemplatetiers.Asset(templatePath)
	require.NoError(t, err)
	expected := templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func expectedTemplateFromBasedOnTierConfig(t *testing.T, tier, templateFileName string) string {
	basedOnTierContent, err := Asset(fmt.Sprintf("%s/based_on_tier.yaml", tier))
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
		require.Len(t, tc.templatesByTier, 3)
		for _, name := range []string{"advanced", "base", "nocluster"} {
			tierData, found := tc.templatesByTier[name]
			tierObjs := tierData.nstmplTierObjs
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
func newNSTemplateTierFromYAML(s *runtime.Scheme, tier, namespace string, deactivationTimeout int, namespaceRevisions map[string]string, clusterResourceQuotaRevision string) (toolchainv1alpha1.NSTemplateTier, string, error) {
	expectedTmpl, err := texttemplate.New("template").Parse(`kind: NSTemplateTier
apiVersion: toolchain.dev.openshift.com/v1alpha1
metadata:
  namespace: {{ .Namespace }}
  name: {{ .Tier }}
spec:
  deactivationTimeoutDays: {{ .DeactivationTimeout }} 
  namespaces: 
{{ $tier := .Tier }}{{ range $kind, $revision := .NamespaceRevisions }}    - templateRef: {{ $tier }}-{{ $kind }}-{{ $revision }}
{{ end }}  clusterResources:
    templateRef: {{ $tier }}-clusterresources-{{ .ClusterResourcesRevision }}`)
	if err != nil {
		return toolchainv1alpha1.NSTemplateTier{}, "", err
	}
	expected := bytes.NewBuffer(nil)
	err = expectedTmpl.Execute(expected, struct {
		Tier                     string
		Namespace                string
		NamespaceRevisions       map[string]string
		ClusterResourcesRevision string
		DeactivationTimeout      int
	}{
		Tier:                     tier,
		Namespace:                namespace,
		NamespaceRevisions:       namespaceRevisions,
		ClusterResourcesRevision: clusterResourceQuotaRevision,
		DeactivationTimeout:      deactivationTimeout,
	})
	if err != nil {
		return toolchainv1alpha1.NSTemplateTier{}, "", err
	}
	result := &toolchainv1alpha1.NSTemplateTier{}
	codecFactory := serializer.NewCodecFactory(s)
	_, _, err = codecFactory.UniversalDeserializer().Decode(expected.Bytes(), nil, result)
	if err != nil {
		return toolchainv1alpha1.NSTemplateTier{}, "", err
	}
	return *result, expected.String(), err
}

func runtimeObjectToNSTemplateTier(t *testing.T, s *runtime.Scheme, tierObj runtime.Object) *toolchainv1alpha1.NSTemplateTier {
	tier := &toolchainv1alpha1.NSTemplateTier{}
	err := s.Convert(tierObj, tier, nil)
	require.NoError(t, err)
	return tier
}
