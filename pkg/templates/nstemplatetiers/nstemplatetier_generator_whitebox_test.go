package nstemplatetiers

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	texttemplate "text/template"

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
			require.Len(t, tmpls, 6)
			require.NotContains(t, "foo", tmpls) // make sure that the `foo: bar` entry was ignored
			for _, tier := range []string{"advanced", "base", "baseextended", "baseextendedidling", "basedeactivationdisabled", "test"} {
				t.Run(tier, func(t *testing.T) {
					for _, kind := range []string{"dev", "stage"} {
						t.Run(kind, func(t *testing.T) {
							switch tier {
							case "base", "test":
								assert.NotEmpty(t, tmpls[tier].rawTemplates.namespaceTemplates[kind].revision)
								assert.NotEmpty(t, tmpls[tier].rawTemplates.namespaceTemplates[kind].content)
							default:
								assert.Empty(t, tmpls[tier].rawTemplates.namespaceTemplates[kind].revision)
								assert.Empty(t, tmpls[tier].rawTemplates.namespaceTemplates[kind].content)
							}
						})
					}
					t.Run("cluster", func(t *testing.T) {
						switch tier {
						case "base", "test":
							require.NotNil(t, tmpls[tier].rawTemplates.clusterTemplate)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.clusterTemplate.revision)
							assert.NotEmpty(t, tmpls[tier].rawTemplates.clusterTemplate.content)
						default:
							assert.Nil(t, tmpls[tier].rawTemplates.clusterTemplate)
						}
					})
					t.Run("based-on-tier", func(t *testing.T) {
						switch tier {
						case "base", "test":
							require.Nil(t, tmpls[tier].basedOnTier)
						default:
							require.NotNil(t, tmpls[tier].basedOnTier)
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
					t.Run("based-on-tier", func(t *testing.T) {
						if tier != "advanced" {
							assert.Empty(t, tmpls[tier].rawTemplates.basedOnTier)
						} else {
							assert.Equal(t, ExpectedRevisions[tier]["based-on-tier"], tmpls[tier].rawTemplates.basedOnTier.revision)
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

			expectedDeactivationTimeoutsByTier := map[string]int{
				"base":                     30,
				"baseextended":             180,
				"baseextendedidling":       30,
				"basedeactivationdisabled": 0,
				"advanced":                 0,
				"test":                     30,
			}

			// when
			// uses the `Asset` funcs generated in the `pkg/templates/nstemplatetiers/` subpackages
			tc, err := newTierGenerator(s, nil, namespace, assets)
			require.NoError(t, err)
			// then
			require.NotEmpty(t, tc.templatesByTier)
			// verify that each NSTemplateTier has the Namespaces and ClusterResources `TemplateRef` set as expected
			for _, tierData := range tc.templatesByTier {

				for _, nstmplTierObj := range tierData.nstmplTierObjs {
					tierObj := nstmplTierObj.GetClientObject()

					// verify tier configuration
					nstmplTier := runtimeObjectToNSTemplateTier(t, s, tierObj)
					require.NotNil(t, nstmplTier)
					assert.Equal(t, namespace, nstmplTier.Namespace)
					expectedDeactivationTimeout, ok := expectedDeactivationTimeoutsByTier[nstmplTier.Name]
					require.True(t, ok, "encountered an unexpected tier")
					assert.Equal(t, expectedDeactivationTimeout, nstmplTier.Spec.DeactivationTimeoutDays)

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
					actual := runtimeObjectToNSTemplateTier(t, s, objects[0].GetClientObject())

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
							switch actual.Spec.Type {
							case "dev", "stage":
								assertNamespaceTemplate(t, decoder, actual.Spec.Template, tier, actual.Spec.Type)
							case "clusterresources":
								assertClusterResourcesTemplate(t, decoder, actual.Spec.Template, tier)
							default:
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
				if name == "metadata.yaml" || strings.HasSuffix(name, "based-on-tier.yaml") {
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
	switch tier {
	case "base", "test":
		expected := templatev1.Template{}
		content, err := Asset(fmt.Sprintf("%s/cluster.yaml", tier))
		require.NoError(t, err)
		_, _, err = decoder.Decode(content, nil, &expected)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	default:
		// todo
	}

	switch tier {
	case "test":
		// skip because this tier is for testing purposes only and the template can change often
	case "base", "baseextended", "basedeactivationdisabled":
		assertDefaultObjects(t, actual, "43200")
	case "baseextendedidling":
		assertDefaultObjects(t, actual, "86400")
	case "advanced":
		assertDefaultObjects(t, actual, "0")
	default:
		t.Errorf("unexpected tier: '%s'", tier)
	}
}

func assertDefaultObjects(t *testing.T, actual templatev1.Template, idlerParamValue string) {
	assert.Len(t, actual.Objects, 13)
	containsObj(t, actual, clusterResourceQuotaComputeObj("20000m", "1750m", "7Gi", "15Gi"))
	containsObj(t, actual, clusterResourceQuotaDeploymentsObj())
	containsObj(t, actual, clusterResourceQuotaReplicasObj())
	containsObj(t, actual, clusterResourceQuotaRoutesObj())
	containsObj(t, actual, clusterResourceQuotaJobsObj())
	containsObj(t, actual, clusterResourceQuotaServicesObj())
	containsObj(t, actual, clusterResourceQuotaBuildConfigObj())
	containsObj(t, actual, clusterResourceQuotaSecretsObj())
	containsObj(t, actual, clusterResourceQuotaConfigMapObj())
	containsObj(t, actual, clusterResourceQuotaRHOASOperatorObj())
	containsObj(t, actual, clusterResourceQuotaSBOObj())
	containsObj(t, actual, idlerObj("${USERNAME}-dev"))
	containsObj(t, actual, idlerObj("${USERNAME}-stage"))
	containsParam(t, actual, "IDLER_TIMEOUT_SECONDS", idlerParamValue)
}

func assertNamespaceTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, tier, kind string) {
	var templatePath string

	switch tier {
	case "base", "test":
		templatePath = fmt.Sprintf("%s/ns_%s.yaml", tier, kind)
	default:
		templatePath = expectedTemplateFromBasedOnTierConfig(t, tier, fmt.Sprintf("ns_%s.yaml", kind))
	}
	content, err := Asset(templatePath)
	require.NoError(t, err)
	expected := templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
	// Assert expected objects in the template
	// Each template should have one Namespace, one RoleBinding, one LimitRange and a varying number of NetworkPolicy objects depending on the namespace kind

	// Template objects count
	switch tier {
	case "advanced", "base", "baseextended", "baseextendedidling", "basedeactivationdisabled":
		if kind == "dev" {
			require.Len(t, actual.Objects, 10) // dev namespace has CRW network policy
		} else {
			require.Len(t, actual.Objects, 9)
		}
	case "test":
		return // Don't care what objects are defined in the test tier
	default:
		require.Fail(t, fmt.Sprintf("unexpected tier: %s", tier))
	}

	// Namespace
	containsObj(t, actual, namespaceObj(kind))

	// RoleBinding "user-edit"
	containsObj(t, actual, userEditRoleBindingObj(kind))

	// LimitRange
	cpuLimit := "1000m"
	memoryLimit := "750Mi"
	memoryRequest := "64Mi"
	cpuRequest := "10m"
	containsObj(t, actual, limitRangeObj(kind, cpuLimit, memoryLimit, cpuRequest, memoryRequest))

	// NetworkPolicies
	containsObj(t, actual, allowSameNamespacePolicyObj(kind))
	containsObj(t, actual, allowFromOpenshiftIngressPolicyObj(kind))
	containsObj(t, actual, allowFromOpenshiftMonitoringPolicyObj(kind))

	// User Namespaces Network Policies
	switch tier {
	case "advanced", "base", "baseextended", "baseextendedidling", "basedeactivationdisabled":
		switch kind {
		case "dev":
			containsObj(t, actual, allowFromCRWPolicyObj(kind))
			containsObj(t, actual, allowOtherNamespacePolicyObj(kind, "stage"))
		case "stage":
			containsObj(t, actual, allowOtherNamespacePolicyObj(kind, "dev"))
		default:
			t.Errorf("unexpected kind: '%s'", kind)
		}
	default:
		t.Errorf("unexpected tier: '%s'", tier)
	}

	// Role & RoleBinding with additional permissions to edit roles/rolebindings
	containsObj(t, actual, rbacEditRoleObj(kind))
	containsObj(t, actual, userRbacEditRoleBindingObj(kind))
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
	cpuLimit := "${CPU_LIMIT}"
	memoryLimit := "7Gi"
	containsObj(t, actual, testClusterResourceQuotaObj(cpuLimit, memoryLimit))
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

	// Namespace
	containsObj(t, actual, testNamespaceObj(kind))
}

func expectedTemplateFromBasedOnTierConfig(t *testing.T, tier, templateFileName string) string {
	basedOnTierContent, err := Asset(fmt.Sprintf("%s/based-on-tier.yaml", tier))
	require.NoError(t, err)
	basedOnTier := BasedOnTier{}
	require.NoError(t, yaml.Unmarshal(basedOnTierContent, &basedOnTier))
	assert.Equal(t, tier, basedOnTier.To.Name)
	return fmt.Sprintf("%s/%s", basedOnTier.From.Name, templateFileName)
}

func containsObj(t *testing.T, template templatev1.Template, obj string) {
	for _, object := range template.Objects {
		if string(object.Raw) == obj {
			return
		}
	}
	assert.Fail(t, "NSTemplateTier doesn't contain the expected object", "Template: %s; \n\nExpected object: %s", template, obj)
}

func containsParam(t *testing.T, template templatev1.Template, name, value string) {
	for _, parameter := range template.Parameters {
		if parameter.Name == name && parameter.Value == value {
			return
		}
	}
	assert.Fail(t, "NSTemplateTier doesn't contain the expected parameter", "Parameters: %v; \n\nExpected parameter: %s with value %s", template.Parameters, name, value)
}

func limitRangeObj(kind, cpuLimit, memoryLimit, cpuRequest, memoryRequest string) string {
	return fmt.Sprintf(`{"apiVersion":"v1","kind":"LimitRange","metadata":{"name":"resource-limits","namespace":"${USERNAME}-%s"},"spec":{"limits":[{"default":{"cpu":"%s","memory":"%s"},"defaultRequest":{"cpu":"%s","memory":"%s"},"type":"Container"}]}}`, kind, cpuLimit, memoryLimit, cpuRequest, memoryRequest)
}

func clusterResourceQuotaComputeObj(cpuLimit, cpuRequest, memoryLimit, storageLimit string) string { // nolint: unparam
	return fmt.Sprintf(`{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-compute"},"spec":{"quota":{"hard":{"count/persistentvolumeclaims":"5","limits.cpu":"%[1]s","limits.ephemeral-storage":"7Gi","limits.memory":"%[3]s","requests.cpu":"%[2]s","requests.ephemeral-storage":"7Gi","requests.memory":"%[3]s","requests.storage":"%[4]s"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`, cpuLimit, cpuRequest, memoryLimit, storageLimit)
}

func clusterResourceQuotaDeploymentsObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-deployments"},"spec":{"quota":{"hard":{"count/deploymentconfigs.apps":"30","count/deployments.apps":"30","count/pods":"50"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func clusterResourceQuotaReplicasObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-replicas"},"spec":{"quota":{"hard":{"count/replicasets.apps":"30","count/replicationcontrollers":"30"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func clusterResourceQuotaRoutesObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-routes"},"spec":{"quota":{"hard":{"count/ingresses.extensions":"10","count/routes.route.openshift.io":"10"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func clusterResourceQuotaJobsObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-jobs"},"spec":{"quota":{"hard":{"count/cronjobs.batch":"30","count/daemonsets.apps":"30","count/jobs.batch":"30","count/statefulsets.apps":"30"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func clusterResourceQuotaServicesObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-services"},"spec":{"quota":{"hard":{"count/services":"10"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func clusterResourceQuotaBuildConfigObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-bc"},"spec":{"quota":{"hard":{"count/buildconfigs.build.openshift.io":"30"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func clusterResourceQuotaSecretsObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-secrets"},"spec":{"quota":{"hard":{"count/secrets":"100"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func clusterResourceQuotaConfigMapObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-cm"},"spec":{"quota":{"hard":{"count/configmaps":"100"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func clusterResourceQuotaRHOASOperatorObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-rhoas"},"spec":{"quota":{"hard":{"count/cloudserviceaccountrequest.rhoas.redhat.com":"2","count/cloudservicesrequests.rhoas.redhat.com":"2","count/kafkaconnections.rhoas.redhat.com":"5"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func clusterResourceQuotaSBOObj() string {
	return `{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}-sbo"},"spec":{"quota":{"hard":{"count/servicebindings.binding.operators.coreos.com":"100"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`
}

func idlerObj(name string) string { //nolint:unparam
	return fmt.Sprintf(`{"apiVersion":"toolchain.dev.openshift.com/v1alpha1","kind":"Idler","metadata":{"name":"%s"},"spec":{"timeoutSeconds":"${{IDLER_TIMEOUT_SECONDS}}"}}`, name)
}

func testClusterResourceQuotaObj(cpuLimit, memoryLimit string) string {
	return fmt.Sprintf(`{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}"},"spec":{"quota":{"hard":{"limits.cpu":"%[1]s","limits.memory":"%[2]s","persistentvolumeclaims":"5","requests.storage":"7Gi"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`, cpuLimit, memoryLimit)
}

func namespaceObj(kind string) string {
	if kind == "code" {
		return fmt.Sprintf(`{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"che.eclipse.org/openshift-username":"${USERNAME}","openshift.io/description":"${USERNAME}-%[1]s","openshift.io/display-name":"${USERNAME}-%[1]s","openshift.io/requester":"${USERNAME}"},"labels":{"name":"${USERNAME}-%[1]s"},"name":"${USERNAME}-%[1]s"}}`, kind)
	}
	return fmt.Sprintf(`{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"openshift.io/description":"${USERNAME}-%[1]s","openshift.io/display-name":"${USERNAME}-%[1]s","openshift.io/requester":"${USERNAME}"},"labels":{"name":"${USERNAME}-%[1]s"},"name":"${USERNAME}-%[1]s"}}`, kind)
}

func testNamespaceObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"openshift.io/description":"${USERNAME}-%[1]s","openshift.io/display-name":"${USERNAME}-%[1]s","openshift.io/requester":"${USERNAME}"},"labels":{"name":"${USERNAME}-%[1]s","toolchain.dev.openshift.com/provider":"codeready-toolchain"},"name":"${USERNAME}-%[1]s"}}`, kind)
}

func userEditRoleBindingObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"name":"user-edit","namespace":"${USERNAME}-%s"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"edit"},"subjects":[{"kind":"User","name":"${USERNAME}"}]}`, kind)
}

func userRbacEditRoleBindingObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"name":"user-rbac-edit","namespace":"${USERNAME}-%s"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"rbac-edit"},"subjects":[{"kind":"User","name":"${USERNAME}"}]}`, kind)
}

func rbacEditRoleObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"name":"rbac-edit","namespace":"${USERNAME}-%s"},"rules":[{"apiGroups":["authorization.openshift.io","rbac.authorization.k8s.io"],"resources":["roles","rolebindings"],"verbs":["get","list","watch","create","update","patch","delete"]}]}`, kind)
}

func allowFromOpenshiftMonitoringPolicyObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"allow-from-openshift-monitoring","namespace":"${USERNAME}-%s"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"network.openshift.io/policy-group":"monitoring"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}`, kind)
}

func allowFromOpenshiftIngressPolicyObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"allow-from-openshift-ingress","namespace":"${USERNAME}-%s"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"network.openshift.io/policy-group":"ingress"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}`, kind)
}

func allowFromCRWPolicyObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"allow-from-codeready-workspaces-operator","namespace":"${USERNAME}-%s"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"network.openshift.io/policy-group":"codeready-workspaces"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}`, kind)
}

func allowSameNamespacePolicyObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"allow-same-namespace","namespace":"${USERNAME}-%s"},"spec":{"ingress":[{"from":[{"podSelector":{}}]}],"podSelector":{}}}`, kind)
}

func allowOtherNamespacePolicyObj(kind string, otherNamespaceKinds ...string) string {
	nsSelectorTmpl := `{"namespaceSelector":{"matchLabels":{"name":"${USERNAME}-%s"}}}`
	var selectors strings.Builder
	for _, other := range otherNamespaceKinds {
		if selectors.Len() > 0 {
			selectors.WriteRune(',')
		}
		selectors.WriteString(fmt.Sprintf(nsSelectorTmpl, other))
	}
	return fmt.Sprintf(`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"allow-from-other-user-namespaces","namespace":"${USERNAME}-%[1]s"},"spec":{"ingress":[{"from":[%[2]s]}],"podSelector":{},"policyTypes":["Ingress"]}}`, kind, selectors.String())
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
			tier := runtimeObjectToNSTemplateTier(t, s, tierObjs[0].GetClientObject())

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
		"based-on-tier": "abcd123",
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
