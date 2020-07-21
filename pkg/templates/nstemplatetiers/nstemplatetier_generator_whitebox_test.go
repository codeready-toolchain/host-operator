package nstemplatetiers

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	texttemplate "text/template"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"

	templatev1 "github.com/openshift/api/template/v1"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestLoadTemplatesByTiers(t *testing.T) {

	logf.SetLogger(zap.Logger(true))

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			assets := assets.NewAssets(AssetNames, Asset)
			// when
			tmpls, err := loadTemplatesByTiers(assets)
			// then
			require.NoError(t, err)
			// then
			require.NoError(t, err)
			require.Len(t, tmpls, 3)
			require.NotContains(t, "foo", tmpls) // make sure that the `foo: bar` entry was ignored
			for _, tier := range []string{"advanced", "basic", "team"} {
				t.Run(tier, func(t *testing.T) {
					for _, kind := range []string{"code", "dev", "stage"} {
						t.Run(kind, func(t *testing.T) {
							if tier == "team" && kind == "code" {
								// not applicable
								return
							}
							assert.NotEmpty(t, tmpls[tier].namespaceTemplates[kind].revision)
							assert.NotEmpty(t, tmpls[tier].namespaceTemplates[kind].content)
						})
					}
					t.Run("cluster", func(t *testing.T) {
						require.NotNil(t, tmpls[tier].clusterTemplate)
						assert.NotEmpty(t, tmpls[tier].clusterTemplate.revision)
						assert.NotEmpty(t, tmpls[tier].clusterTemplate.content)
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

			for _, tier := range []string{"advanced", "basic", "team", "nocluster"} {
				t.Run(tier, func(t *testing.T) {
					for _, kind := range []string{"code", "dev", "stage"} {
						t.Run(kind, func(t *testing.T) {
							if tier == "team" && kind == "code" {
								// not applicable
								return
							}
							assert.Equal(t, ExpectedRevisions[tier][kind], tmpls[tier].namespaceTemplates[kind].revision)
							assert.NotEmpty(t, tmpls[tier].namespaceTemplates[kind].content)
						})
					}
					t.Run("cluster", func(t *testing.T) {
						if tier == "nocluster" {
							assert.Nil(t, tmpls[tier].clusterTemplate)
							return
						}
						require.NotNil(t, tmpls[tier].clusterTemplate)
						assert.Equal(t, ExpectedRevisions[tier]["cluster"], tmpls[tier].clusterTemplate.revision)
						assert.NotEmpty(t, tmpls[tier].clusterTemplate.content)
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
			namespace := "host-operator-" + uuid.NewV4().String()[:7]
			assets := assets.NewAssets(AssetNames, Asset)
			// uses the `Asset` funcs generated in the `pkg/templates/nstemplatetiers/` subpackages
			tierTmpls, err := newTierTemplates(s, namespace, assets)
			require.NoError(t, err)
			// when
			nstmplTiers, err := newNSTemplateTiers(namespace, tierTmpls)
			// then
			require.NoError(t, err)
			require.NotEmpty(t, nstmplTiers)
			// verify that each NSTemplateTier has the Namespaces and ClusterResources `TemplateRef` set as expected
			for _, nstmplTier := range nstmplTiers {
				tier := nstmplTier.Name
				assert.Equal(t, namespace, nstmplTier.Namespace)
				require.Len(t, nstmplTier.Spec.Namespaces, len(tierTmpls[tier])-1) // exclude clusterresources TierTemplate here
				expectedTierTmplNames := make([]string, 0, len(tierTmpls[tier])-1)
				var clusterResourcesTemplateRef string
				for _, tierTmpl := range tierTmpls[tier] {
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
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			namespace := "host-operator-" + uuid.NewV4().String()[:7]
			assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
			tierTmpls, err := newTierTemplates(s, namespace, assets)
			require.NoError(t, err)
			namespaceRevisions := map[string]map[string]string{
				"advanced": {
					"code":  "123456a",
					"dev":   "123456b",
					"stage": "123456c",
				},
				"basic": {
					"code":  "123456d",
					"dev":   "123456e",
					"stage": "123456f",
				},
			}
			clusterResourceQuotaRevisions := map[string]string{
				"advanced": "654321a",
				"basic":    "654321b",
			}
			for tier := range namespaceRevisions {
				t.Run(tier, func(t *testing.T) {
					// given
					tmpls := tierTmpls[tier]
					// when
					actual, err := newNSTemplateTier(namespace, tier, tmpls)
					// then
					require.NoError(t, err)
					expected, _, err := newNSTemplateTierFromYAML(s, tier, namespace, namespaceRevisions[tier], clusterResourceQuotaRevisions[tier])
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
	namespace := "host-operator-" + uuid.NewV4().String()[:7]

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			assets := assets.NewAssets(AssetNames, Asset)
			// uses the `Asset` funcs generated in the `pkg/templates/nstemplatetiers/` subpackages
			// when
			tierTmpls, err := newTierTemplates(s, namespace, assets)
			// then
			require.NoError(t, err)
			decoder := serializer.NewCodecFactory(s).UniversalDeserializer()

			resourceNameRE, err := regexp.Compile(`[a-z0-9\.-]+`)
			require.NoError(t, err)
			for tier, tmpls := range tierTmpls {
				t.Run(tier, func(t *testing.T) {
					for _, actual := range tmpls {
						t.Run(actual.Name, func(t *testing.T) {
							assert.Equal(t, namespace, actual.Namespace)
							assert.True(t, resourceNameRE.MatchString(actual.Name)) // verifies that the TierTemplate name complies with the DNS-1123 spec
							assert.NotEmpty(t, actual.Spec.Revision)
							assert.NotEmpty(t, actual.Spec.TierName)
							assert.NotEmpty(t, actual.Spec.Type)
							assert.NotEmpty(t, actual.Spec.Template)
							switch actual.Spec.Type {
							case "dev", "code", "stage":
								assertNamespaceTemplate(t, decoder, actual.Spec.Template, actual.Spec.TierName, actual.Spec.Type)
							case "clusterresources":
								assertClusterResourcesTemplate(t, decoder, actual.Spec.Template, actual.Spec.TierName)
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
			tierTmpls, err := newTierTemplates(s, namespace, assets)
			// then
			require.NoError(t, err)
			decoder := serializer.NewCodecFactory(s).UniversalDeserializer()

			resourceNameRE, err := regexp.Compile(`[a-z0-9\.-]+`)
			require.NoError(t, err)
			for tier, tmpls := range tierTmpls {
				t.Run(tier, func(t *testing.T) {
					for _, actual := range tmpls {
						assert.Equal(t, namespace, actual.Namespace)
						assert.True(t, resourceNameRE.MatchString(actual.Name)) // verifies that the TierTemplate name complies with the DNS-1123 spec
						assert.NotEmpty(t, actual.Spec.Revision)
						assert.NotEmpty(t, actual.Spec.TierName)
						assert.NotEmpty(t, actual.Spec.Type)
						assert.NotEmpty(t, actual.Spec.Template)
						switch actual.Spec.Type {
						case "dev", "code", "stage":
							assertTestNamespaceTemplate(t, decoder, actual.Spec.Template, actual.Spec.TierName, actual.Spec.Type)
						case "clusterresources":
							assertTestClusteResourcesTemplate(t, decoder, actual.Spec.Template, actual.Spec.TierName)
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
				if name == "metadata.yaml" {
					return testnstemplatetiers.Asset(name)
				}
				// error occurs when fetching the content of the 'advanced-code.yaml' template
				return []byte("invalid"), nil // return an invalid YAML represention of a Template
			})
			// when
			_, err = newTierTemplates(s, namespace, fakeAssets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to generate 'advanced-code-123456a' TierTemplate manifest: couldn't get version/kind; json parse error")
		})
	})
}

func assertClusterResourcesTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, tier string) {
	expected := templatev1.Template{}
	content, err := Asset(fmt.Sprintf("%s/cluster.yaml", tier))
	require.NoError(t, err)
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
	if tier == "team" {
		assert.Len(t, actual.Objects, 3) // No Idler for -code namespace (there is no -code namespace in team tier)
	} else {
		assert.Len(t, actual.Objects, 4)
	}
	cpuLimit := "4000m"
	cpuRequest := "1750m"
	memoryLimit := "7Gi"
	if tier == "team" {
		cpuRequest = "2000m"
		memoryLimit = "15Gi"
	}
	containsObj(t, actual, clusterResourceQuotaObj(cpuLimit, cpuRequest, memoryLimit))
	containsObj(t, actual, idlerObj("${USERNAME}-dev", "28800"))
	if tier != "team" {
		containsObj(t, actual, idlerObj("${USERNAME}-code", "28800"))
	}
	containsObj(t, actual, idlerObj("${USERNAME}-stage", "28800"))
}

func assertNamespaceTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, tier, kind string) {
	content, err := Asset(fmt.Sprintf("%s/ns_%s.yaml", tier, kind))
	require.NoError(t, err)
	expected := templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
	// Assert expected objects in the template
	// Each template should have one Namespace, one RoleBinding, one LimitRange and three NetworkPolicy objects

	// Namespace
	containsObj(t, actual, namespaceObj(kind))
	// RoleBinding "user-edit"
	containsObj(t, actual, userEditRoleBindingObj(kind))

	// LimitRange
	cpuLimit := "150m"
	memoryLimit := "512Mi"
	memoryRequest := "64Mi"
	cpuRequest := "10m"
	if kind == "code" {
		cpuLimit = "1000m"
		cpuRequest = "60m"
		memoryRequest = "307Mi"
	}
	if tier == "team" {
		memoryLimit = "1Gi"
	} else if kind == "dev" {
		memoryLimit = "750Mi"
	}
	containsObj(t, actual, limitRangeObj(kind, cpuLimit, memoryLimit, cpuRequest, memoryRequest))

	// NetworkPolicies
	containsObj(t, actual, allowSameNamespacePolicyObj(kind))
	containsObj(t, actual, allowFromOpenshiftIngressPolicyObj(kind))
	containsObj(t, actual, allowFromOpenshiftMonitoringPolicyObj(kind))

	// All templates in the "team" tier and "-code" templates in other tiers should also have additional RoleBinding and Role
	if kind == "code" || tier == "team" || tier == "advanced" {
		require.Len(t, actual.Objects, 8)
		// Role & RoleBinding with additional permissions to edit roles/rolebindings
		containsObj(t, actual, rbacEditRoleObj(kind))
		containsObj(t, actual, userRbacEditRoleBindingObj(kind))
	} else {
		require.Len(t, actual.Objects, 6)
	}
}

func assertTestClusteResourcesTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, tier string) {
	content, err := testnstemplatetiers.Asset(fmt.Sprintf("%s/cluster.yaml", tier))
	require.NoError(t, err)
	expected := templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
	cpuLimit := "4000m"
	memoryLimit := "7Gi"
	if tier == "team" {
		cpuLimit = "2000m"
	}
	containsObj(t, actual, testClusterResourceQuotaObj(cpuLimit, memoryLimit))
}

func assertTestNamespaceTemplate(t *testing.T, decoder runtime.Decoder, actual templatev1.Template, tier, kind string) {
	content, err := testnstemplatetiers.Asset(fmt.Sprintf("%s/ns_%s.yaml", tier, kind))
	require.NoError(t, err)
	expected := templatev1.Template{}
	_, _, err = decoder.Decode(content, nil, &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)

	// Namespace
	containsObj(t, actual, testNamespaceObj(kind))
}

func containsObj(t *testing.T, template templatev1.Template, obj string) {
	for _, object := range template.Objects {
		if string(object.Raw) == obj {
			return
		}
	}
	assert.Fail(t, "NSTemplateTier doesn't contain the expected object", "Template: %s; \n\nExpected object: %s", template, obj)
}

func limitRangeObj(kind, cpuLimit, memoryLimit, cpuRequest, memoryRequest string) string {
	return fmt.Sprintf(`{"apiVersion":"v1","kind":"LimitRange","metadata":{"name":"resource-limits","namespace":"${USERNAME}-%s"},"spec":{"limits":[{"default":{"cpu":"%s","memory":"%s"},"defaultRequest":{"cpu":"%s","memory":"%s"},"type":"Container"}]}}`, kind, cpuLimit, memoryLimit, cpuRequest, memoryRequest)
}

func clusterResourceQuotaObj(cpuLimit, cpuRequest, memoryLimit string) string {
	return fmt.Sprintf(`{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}"},"spec":{"quota":{"hard":{"configmaps":"100","limits.cpu":"%[1]s","limits.ephemeral-storage":"7Gi","limits.memory":"%[3]s","persistentvolumeclaims":"5","pods":"100","replicationcontrollers":"100","requests.cpu":"%[2]s","requests.ephemeral-storage":"7Gi","requests.memory":"%[3]s","requests.storage":"7Gi","secrets":"100","services":"100"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`, cpuLimit, cpuRequest, memoryLimit)
}

func idlerObj(name, timeout string) string {
	return fmt.Sprintf(`{"apiVersion":"toolchain.dev.openshift.com/v1alpha1","kind":"Idler","metadata":{"name":"%s"},"spec":{"timeoutSeconds":%s}}`, name, timeout)
}

func testClusterResourceQuotaObj(cpuLimit, memoryLimit string) string {
	return fmt.Sprintf(`{"apiVersion":"quota.openshift.io/v1","kind":"ClusterResourceQuota","metadata":{"name":"for-${USERNAME}"},"spec":{"quota":{"hard":{"limits.cpu":"%[1]s","limits.memory":"%[2]s","persistentvolumeclaims":"5","requests.storage":"7Gi"}},"selector":{"annotations":{"openshift.io/requester":"${USERNAME}"},"labels":null}}}`, cpuLimit, memoryLimit)
}

func namespaceObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"openshift.io/description":"${USERNAME}-%[1]s","openshift.io/display-name":"${USERNAME}-%[1]s","openshift.io/requester":"${USERNAME}"},"name":"${USERNAME}-%[1]s"}}`, kind)
}

func testNamespaceObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"openshift.io/description":"${USERNAME}-%[1]s","openshift.io/display-name":"${USERNAME}-%[1]s","openshift.io/requester":"${USERNAME}"},"labels":{"toolchain.dev.openshift.com/provider":"codeready-toolchain"},"name":"${USERNAME}-%[1]s"}}`, kind)
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

func allowSameNamespacePolicyObj(kind string) string {
	return fmt.Sprintf(`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"allow-same-namespace","namespace":"${USERNAME}-%s"},"spec":{"ingress":[{"from":[{"podSelector":{}}]}],"podSelector":{}}}`, kind)
}

func TestNewNSTemplateTiers(t *testing.T) {

	// given
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		// given
		namespace := "host-operator-" + uuid.NewV4().String()[:7]
		assets := assets.NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
		tierTmpls, err := newTierTemplates(s, namespace, assets)
		require.NoError(t, err)
		// when
		tiers, err := newNSTemplateTiers(namespace, tierTmpls)
		// then
		require.NoError(t, err)
		require.Len(t, tiers, 4)
		for _, name := range []string{"advanced", "basic", "team", "nocluster"} {
			tier, found := tiers[name]
			require.True(t, found)
			assert.Equal(t, name, tier.ObjectMeta.Name)
			assert.Equal(t, namespace, tier.ObjectMeta.Namespace)
			for _, ns := range tier.Spec.Namespaces {
				assert.NotEmpty(t, ns.TemplateRef)
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
		"code":    "123456a",
		"dev":     "123456b",
		"stage":   "123456c",
		"cluster": "654321a",
	},
	"basic": {
		"code":    "123456d",
		"dev":     "123456e",
		"stage":   "123456f",
		"cluster": "654321b",
	},
	"team": {
		"dev":     "123456g",
		"stage":   "123456h",
		"cluster": "654321c",
	},
	"nocluster": {
		"code":  "123456i",
		"dev":   "123456j",
		"stage": "1234567",
	},
}

// newNSTemplateTierFromYAML generates toolchainv1alpha1.NSTemplateTier using a golang template which is applied to the given tier.
func newNSTemplateTierFromYAML(s *runtime.Scheme, tier, namespace string, namespaceRevisions map[string]string, clusterResourceQuotaRevision string) (toolchainv1alpha1.NSTemplateTier, string, error) {
	expectedTmpl, err := texttemplate.New("template").Parse(`kind: NSTemplateTier
apiVersion: toolchain.dev.openshift.com/v1alpha1
metadata:
  namespace: {{ .Namespace }}
  name: {{ .Tier }}
spec:
  namespaces: 
{{ $tier := .Tier }}{{ range $kind, $revision := .NamespaceRevisions }}  - type: {{ $kind }}
    revision: "{{ $revision }}"
    template:
      apiVersion: template.openshift.io/v1
      kind: Template
      metadata:
        labels:
          toolchain.dev.openshift.com/provider: codeready-toolchain
        name: {{ $tier }}-{{ $kind }}
      objects:
      - apiVersion: v1
        kind: Namespace
        metadata:
          annotations:
            openshift.io/description: ${USERNAME}-{{ $kind }}
            openshift.io/display-name: ${USERNAME}-{{ $kind }}
            openshift.io/requester: ${USERNAME}
          labels:
            toolchain.dev.openshift.com/provider: codeready-toolchain
          name: ${USERNAME}-{{ $kind }}
      parameters:
      - name: USERNAME
        required: true
    templateRef: {{ $tier }}-{{ $kind }}-{{ $revision }}
{{ end }}  clusterResources:
    revision: "{{ .ClusterResourcesRevision }}"
    template:
      apiVersion: template.openshift.io/v1
      kind: Template
      metadata:
        name: {{ .Tier }}-cluster-resources
        labels:
          toolchain.dev.openshift.com/provider: codeready-toolchain
      objects:
      - apiVersion: quota.openshift.io/v1
        kind: ClusterResourceQuota
        metadata:
          name: for-${USERNAME}
        spec:
          quota: 
            hard:
              limits.cpu: 4000m
              limits.memory: 7Gi
              requests.storage: 7Gi
              persistentvolumeclaims: "5"
          selector:
            annotations:
              openshift.io/requester: ${USERNAME}
            labels: null
      parameters:
      - name: USERNAME
        required: true
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
	}{
		Tier:                     tier,
		Namespace:                namespace,
		NamespaceRevisions:       namespaceRevisions,
		ClusterResourcesRevision: clusterResourceQuotaRevision,
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
