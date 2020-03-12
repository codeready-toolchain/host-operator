package nstemplatetiers

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	texttemplate "text/template"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"

	templatev1 "github.com/openshift/api/template/v1"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestLoadTemplatesByTiers(t *testing.T) {

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			assets := NewAssets(AssetNames, Asset)
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
						t.Run("cluster", func(t *testing.T) {
							require.NotNil(t, tmpls[tier].clusterTemplate)
							assert.NotEmpty(t, tmpls[tier].clusterTemplate.revision)
							assert.NotEmpty(t, tmpls[tier].clusterTemplate.content)
						})
					}
				})
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			assets := NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
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
			assets := NewAssets(testnstemplatetiers.AssetNames, fakeAssets)
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
			assets := NewAssets(testnstemplatetiers.AssetNames, fakeAssets)
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
			assets := NewAssets(testnstemplatetiers.AssetNames, fakeAssets)
			// when
			_, err := loadTemplatesByTiers(assets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load templates: an error occurred")
		})
	})
}

func TestNewNSTemplateTier(t *testing.T) {

	s := scheme.Scheme
	decoder := serializer.NewCodecFactory(s).UniversalDeserializer()
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			assets := NewAssets(AssetNames, Asset)
			// uses the `Asset` funcs generated in the `pkg/templates/nstemplatetiers/` subpackages
			templatesByTier, err := loadTemplatesByTiers(assets)
			require.NoError(t, err)
			namespace := "host-operator-" + uuid.NewV4().String()[:7]
			// when
			tierObjs, err := newNSTemplateTiers(s, namespace, templatesByTier)
			// then
			require.NoError(t, err)
			require.NotEmpty(t, tierObjs)
			decoder := serializer.NewCodecFactory(s).UniversalDeserializer()
			for _, actual := range tierObjs {
				tier := actual.Name
				assert.Equal(t, namespace, actual.Namespace)
				require.Len(t, actual.Spec.Namespaces, len(templatesByTier[tier].namespaceTemplates))
				for kind, tmpl := range templatesByTier[tier].namespaceTemplates {
					found := false
					for _, ns := range actual.Spec.Namespaces {
						if ns.Type != kind {
							continue
						}
						found = true
						assert.Equal(t, tmpl.revision, ns.Revision)
						content, err := Asset(fmt.Sprintf("%s/ns_%s.yaml", tier, kind))
						require.NoError(t, err)
						tmplObj := &templatev1.Template{}
						_, _, err = decoder.Decode(content, nil, tmplObj)
						require.NoError(t, err)
						assert.Equal(t, *tmplObj, ns.Template)

						// Assert expected objects in the template
						// Each template should have one Namespace and one RoleBinding object
						// except "code" which should also have additional RoleBinding and Role
						if kind == "code" || tier == "team" {
							require.Len(t, ns.Template.Objects, 4)
						} else {
							require.Len(t, ns.Template.Objects, 2)
						}
						rbFound := false
						for _, object := range ns.Template.Objects {
							if strings.Contains(string(object.Raw), `"kind":"RoleBinding","metadata":{"labels":{"provider":"codeready-toolchain"},"name":"user-edit"`) {
								rbFound = true
								break
							}
						}
						assert.True(t, rbFound, "the user-edit RoleBinding wasn't found in the namespace of the type", "ns-type", kind)
						break
					}
					assert.True(t, found, "the namespace with the type wasn't found", "ns-type", kind)
				}
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			assets := NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
			templatesByTier, err := loadTemplatesByTiers(assets)
			require.NoError(t, err)
			namespace := "host-operator-" + uuid.NewV4().String()[:7]
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
					tmpls := templatesByTier[tier]
					// when
					actual, err := newNSTemplateTier(decoder, namespace, tier, *tmpls)
					// then
					require.NoError(t, err)
					expected, _, err := newNSTemplateTierFromYAML(s, tier, namespace, namespaceRevisions[tier], clusterResourceQuotaRevisions[tier])
					require.NoError(t, err)
					// here we don't compare whoe objects because the generated NSTemplateTier
					// has no specific values for the `TypeMeta`: the `APIVersion: toolchain.dev.openshift.com/v1alpha1`
					// and `Kind: NSTemplateTier` should be set by the client using the registered GVK
					assert.Equal(t, expected.ObjectMeta, actual.ObjectMeta)
					assert.Equal(t, expected.Spec, actual.Spec)
				})
			}
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("invalid template", func(t *testing.T) {
			// given
			fakeAssets := NewAssets(testnstemplatetiers.AssetNames, func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return testnstemplatetiers.Asset(name)
				}
				// error occurs when fetching the content of the 'advanced-code.yaml' template
				return []byte("invalid"), nil // return an invalid YAML represention of a Template
			})
			namespace := "host-operator-" + uuid.NewV4().String()[:7]
			templatesByTier, err := loadTemplatesByTiers(fakeAssets)
			require.NoError(t, err)
			advancedTemplates := templatesByTier["advanced"]
			// when
			_, err = newNSTemplateTier(decoder, namespace, "advanced", *advancedTemplates)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to generate 'advanced' NSTemplateTier manifest: couldn't get version/kind; json parse error")
		})
	})
}

func TestNewNSTemplateTiers(t *testing.T) {

	// given
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		// given
		assets := NewAssets(testnstemplatetiers.AssetNames, testnstemplatetiers.Asset)
		templatesByTier, err := loadTemplatesByTiers(assets)
		require.NoError(t, err)
		namespace := "host-operator-" + uuid.NewV4().String()[:7]
		// when
		tiers, err := newNSTemplateTiers(s, namespace, templatesByTier)
		// then
		require.NoError(t, err)
		require.Len(t, tiers, 4)
		for _, name := range []string{"advanced", "basic", "team", "nocluster"} {
			tier, found := tiers[name]
			require.True(t, found)
			assert.Equal(t, name, tier.ObjectMeta.Name)
			assert.Equal(t, namespace, tier.ObjectMeta.Namespace)
			for _, ns := range tier.Spec.Namespaces {
				assert.NotEmpty(t, ns.Revision)
				assert.NotEmpty(t, ns.Type)
				assert.NotEmpty(t, ns.Template)
			}
			if name == "nocluster" {
				assert.Nil(t, tier.Spec.ClusterResources)
			} else {
				require.NotNil(t, tier.Spec.ClusterResources)
				assert.NotEmpty(t, tier.Spec.ClusterResources.Revision)
				assert.NotEmpty(t, tier.Spec.ClusterResources.Template)
			}
		}
	})
}

var ExpectedRevisions = map[string]map[string]string{
	"advanced": map[string]string{
		"code":    "123456a",
		"dev":     "123456b",
		"stage":   "123456c",
		"cluster": "654321a",
	},
	"basic": map[string]string{
		"code":    "123456d",
		"dev":     "123456e",
		"stage":   "123456f",
		"cluster": "654321b",
	},
	"team": map[string]string{
		"dev":     "123456g",
		"stage":   "123456h",
		"cluster": "654321c",
	},
	"nocluster": map[string]string{
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
{{ $tier := .Tier }}{{ range $kind, $namespaceRevision := .NamespaceRevisions }}  - type: {{ $kind }}
    revision: "{{ $namespaceRevision }}"
    template:
      apiVersion: template.openshift.io/v1
      kind: Template
      metadata:
        labels:
          provider: codeready-toolchain
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
            provider: codeready-toolchain
          name: ${USERNAME}-{{ $kind }}
      parameters:
      - name: USERNAME
        required: true
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
              limits.cpu: 1750m
              limits.memory: 7Gi
              requests.storage: 5Gi
              persistentvolumeclaims: "2"
          selector:
            annotations:
              openshift.io/requester: ${USERNAME}
            labels: null
      parameters:
      - name: USERNAME
        required: true`)
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
