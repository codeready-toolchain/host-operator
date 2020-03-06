package nstemplatetiers

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	texttemplate "text/template"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers/clusterresources"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers/namespaces"
	testclusterresources "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers/clusterresources"
	testnamespaces "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers/namespaces"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestNewNSTemplateTierGenerator(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		// when initializing the generator with the production Asset
		g, err := newNSTemplateTierGenerator(s, testnamespaces.Asset, testclusterresources.Asset)
		// then
		require.NoError(t, err)
		// verify that there are namespace templates+revisions for the tiers/types
		require.Len(t, g.namespaces, 2)
		require.NotContains(t, "foo", g.namespaces) // make sure that the `foo: bar` entry was ignored
		assert.Equal(t, "123456a", g.namespaces["advanced"]["code"].revision)
		assert.NotEmpty(t, "123456a", g.namespaces["advanced"]["code"].content)
		assert.Equal(t, "123456b", g.namespaces["advanced"]["dev"].revision)
		assert.NotEmpty(t, "123456a", g.namespaces["advanced"]["dev"].content)
		assert.Equal(t, "123456c", g.namespaces["advanced"]["stage"].revision)
		assert.NotEmpty(t, "123456a", g.namespaces["advanced"]["stage"].content)
		assert.Equal(t, "123456d", g.namespaces["basic"]["code"].revision)
		assert.NotEmpty(t, "123456a", g.namespaces["basic"]["code"].content)
		assert.Equal(t, "123456e", g.namespaces["basic"]["dev"].revision)
		assert.NotEmpty(t, "123456a", g.namespaces["basic"]["dev"].content)
		assert.Equal(t, "1234567", g.namespaces["basic"]["stage"].revision)
		assert.NotEmpty(t, "123456a", g.namespaces["basic"]["stage"].content)
		// verify that there are cluster resource quota templates+revisions for the tiers
		require.Len(t, g.clusterResources, 2)
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("failed to initialize generator", func(t *testing.T) {

			t.Run("error with namespace assets", func(t *testing.T) {
				// given
				fakeAssets := func(name string) ([]byte, error) {
					return nil, errors.Errorf("an error")
				}
				// when
				_, err := newNSTemplateTierGenerator(s, fakeAssets, testclusterresources.Asset)
				// then
				require.Error(t, err)
				assert.Equal(t, "unable to load namespace templates: an error", err.Error())
			})

			t.Run("error with cluster resource quota assets", func(t *testing.T) {
				// given
				fakeAssets := func(name string) ([]byte, error) {
					return nil, errors.Errorf("an error")
				}
				// when
				_, err := newNSTemplateTierGenerator(s, testnamespaces.Asset, fakeAssets)
				// then
				require.Error(t, err)
				assert.Equal(t, "unable to load cluster resource quota templates: an error", err.Error())
			})
		})

		t.Run("unparseable content", func(t *testing.T) {

			t.Run("error with namespace assets", func(t *testing.T) {
				// given
				fakeAssets := func(name string) ([]byte, error) {
					return []byte("foo::bar"), nil
				}
				// when
				_, err := newNSTemplateTierGenerator(s, fakeAssets, testclusterresources.Asset)
				// then
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unable to load namespace templates: yaml: unmarshal errors")
			})

			t.Run("error with cluster resource quota assets", func(t *testing.T) {
				// given
				fakeAssets := func(name string) ([]byte, error) {
					return []byte("foo::bar"), nil
				}
				// when
				_, err := newNSTemplateTierGenerator(s, testnamespaces.Asset, fakeAssets)
				// then
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unable to load cluster resource quota templates: yaml: unmarshal errors:")
			})
		})

		t.Run("unknown metadata.yaml", func(t *testing.T) {

			t.Run("error with namespace assets", func(t *testing.T) {
				// given
				fakeAssets := func(name string) ([]byte, error) {
					return nil, errors.Errorf("Asset %s not found", name)
				}
				// when
				_, err := newNSTemplateTierGenerator(s, fakeAssets, testclusterresources.Asset)
				// then
				require.Error(t, err)
				assert.Equal(t, "unable to load namespace templates: Asset metadata.yaml not found", err.Error())
			})

			t.Run("error with cluster resource quota assets", func(t *testing.T) {
				// given
				fakeAssets := func(name string) ([]byte, error) {
					return nil, errors.Errorf("Asset %s not found", name)
				}
				// when
				_, err := newNSTemplateTierGenerator(s, testnamespaces.Asset, fakeAssets)
				// then
				require.Error(t, err)
				assert.Equal(t, "unable to load cluster resource quota templates: Asset metadata.yaml not found", err.Error())
			})

		})

		t.Run("unknown asset", func(t *testing.T) {

			t.Run("unknown namespace template file", func(t *testing.T) {
				// given
				fakeAsset := func(name string) ([]byte, error) {
					if name == "metadata.yaml" {
						return []byte("unknown-asset: 123456a"), nil
					}
					return nil, errors.Errorf("Asset '%s' not found", name)
				}
				// when
				_, err := newNSTemplateTierGenerator(s, fakeAsset, testclusterresources.Asset)
				// then
				require.Error(t, err)
				assert.Equal(t, "unable to load namespace templates: Asset 'unknown-asset.yaml' not found", err.Error())
			})

			t.Run("unknown cluster resource quota template file", func(t *testing.T) {
				// given
				fakeAsset := func(name string) ([]byte, error) {
					if name == "metadata.yaml" {
						return []byte("unknown-asset: 123456a"), nil
					}
					return nil, errors.Errorf("Asset '%s' not found", name)
				}
				// when
				_, err := newNSTemplateTierGenerator(s, testnamespaces.Asset, fakeAsset)
				// then
				require.Error(t, err)
				assert.Equal(t, err.Error(), "unable to load cluster resource quota templates: Asset 'unknown-asset.yaml' not found")
			})
		})
	})
}

func TestLoadNamespaceAssets(t *testing.T) {

	t.Run("ok", func(t *testing.T) {
		// when
		assets, err := loadNamespaceAssets(testnamespaces.Asset)
		// then
		require.NoError(t, err)
		require.Len(t, assets, 2)
		require.NotContains(t, "foo", assets) // make sure that the `foo: bar` entry was ignored
		assert.Equal(t, "123456a", assets["advanced"]["code"].revision)
		assert.NotEmpty(t, "123456a", assets["advanced"]["code"].content)
		assert.Equal(t, "123456b", assets["advanced"]["dev"].revision)
		assert.NotEmpty(t, "123456a", assets["advanced"]["dev"].content)
		assert.Equal(t, "123456c", assets["advanced"]["stage"].revision)
		assert.NotEmpty(t, "123456a", assets["advanced"]["stage"].content)
		assert.Equal(t, "123456d", assets["basic"]["code"].revision)
		assert.NotEmpty(t, "123456a", assets["basic"]["code"].content)
		assert.Equal(t, "123456e", assets["basic"]["dev"].revision)
		assert.NotEmpty(t, "123456a", assets["basic"]["dev"].content)
		assert.Equal(t, "1234567", assets["basic"]["stage"].revision)
		assert.NotEmpty(t, "123456a", assets["basic"]["stage"].content)
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unparseable content", func(t *testing.T) {
			// given
			fakeAssets := func(name string) ([]byte, error) {
				return []byte("foo::bar"), nil
			}
			// when initializing the generator with the production Asset
			_, err := loadNamespaceAssets(fakeAssets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load namespace templates: yaml: unmarshal errors:")
		})

		t.Run("invalid key format", func(t *testing.T) {
			// given
			fakeAssets := func(name string) ([]byte, error) {
				return []byte("foo: bar"), nil
			}
			// when initializing the generator with the production Asset
			_, err := loadNamespaceAssets(fakeAssets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid namespace template filename. Expected format: '<tier_kind>-<namespace_kind>', got foo")
		})
	})
}

func TestLoadClusterResourceQuotaAssets(t *testing.T) {

	t.Run("ok", func(t *testing.T) {
		// when
		assets, err := loadClusterResourceAssets(testclusterresources.Asset)
		// then
		require.NoError(t, err)
		require.Len(t, assets, 2)
		assert.Equal(t, "654321a", assets["advanced"].revision)
		assert.Equal(t, "654321b", assets["basic"].revision)
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unparseable content", func(t *testing.T) {
			// given
			fakeAssets := func(name string) ([]byte, error) {
				return []byte("foo::bar"), nil
			}
			// when initializing the generator with the production Asset
			_, err := loadClusterResourceAssets(fakeAssets)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to load cluster resource quota templates: yaml: unmarshal errors:")
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
			// uses the `Asset` funcs generated in the `pkg/templates/nstemplatetiers/` subpackages
			g, err := newNSTemplateTierGenerator(s, namespaces.Asset, clusterresources.Asset)
			require.NoError(t, err)
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			// when
			tiers, err := g.newNSTemplateTiers(namespace)
			// then
			require.NoError(t, err)
			require.NotEmpty(t, tiers)
			decoder := serializer.NewCodecFactory(s).UniversalDeserializer()
			for _, actual := range tiers {
				tier := actual.Name
				assert.Equal(t, namespace, actual.Namespace)
				require.Len(t, g.namespaces[tier], len(actual.Spec.Namespaces))
				for typ, asset := range g.namespaces[tier] {
					found := false
					for _, ns := range actual.Spec.Namespaces {
						if ns.Type == typ {
							found = true
							assert.Equal(t, asset.revision, ns.Revision)
							content, err := namespaces.Asset(fmt.Sprintf("%s-%s.yaml", tier, typ))
							require.NoError(t, err)
							tmplObj := &templatev1.Template{}
							_, _, err = decoder.Decode(content, nil, tmplObj)
							require.NoError(t, err)
							assert.Equal(t, *tmplObj, ns.Template)

							// Assert expected objects in the template
							// Each template should have one Namespace and one RoleBinding object
							// except "code" which should also have additional RoleBinding and Role
							if typ == "code" || tier == "team" {
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
							assert.True(t, rbFound, "the user-edit RoleBinding wasn't found in the namespace of the type", "ns-type", typ)
							break
						}
					}
					assert.True(t, found, "the namespace with the type wasn't found", "ns-type", typ)
				}
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			// uses the `Asset` funcs generated in the `test/templates/nstemplatetiers/` subpackages
			g, err := newNSTemplateTierGenerator(s, testnamespaces.Asset, testclusterresources.Asset)
			require.NoError(t, err)
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			namespaceRevisions := map[string]map[string]string{
				"advanced": {
					"code":  "123456a",
					"dev":   "123456b",
					"stage": "123456c",
				},
				"basic": {
					"code":  "123456d",
					"dev":   "123456e",
					"stage": "1234567",
				},
			}
			clusterResourceQuotaRevisions := map[string]string{
				"advanced": "654321a",
				"basic":    "654321b",
			}
			for tier := range namespaceRevisions {
				t.Run(tier, func(t *testing.T) {
					// when
					actual, err := g.newNSTemplateTier(tier, namespace)
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

		t.Run("unknown tier", func(t *testing.T) {
			// given
			// uses the `Asset` funcs generated in the `test/templates/nstemplatetiers/` subpackages
			g, err := newNSTemplateTierGenerator(s, testnamespaces.Asset, testclusterresources.Asset)
			require.NoError(t, err)
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			// when
			_, err = g.newNSTemplateTier("foo", namespace)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tier 'foo' does not exist")
		})
	})
}

func TestNewNSTemplateTiers(t *testing.T) {

	// given
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		// uses the `Asset` funcs generated in the `test/templates/nstemplatetiers/` subpackages
		g, err := newNSTemplateTierGenerator(s, testnamespaces.Asset, testclusterresources.Asset)
		require.NoError(t, err)
		namespace := "host-operator" + uuid.NewV4().String()[:7]

		// when
		tiers, err := g.newNSTemplateTiers(namespace)
		// then
		require.NoError(t, err)
		require.Len(t, tiers, 2)
		advancedTier, found := tiers["advanced"]
		require.True(t, found)
		assert.Equal(t, "advanced", advancedTier.ObjectMeta.Name)
		assert.Equal(t, namespace, advancedTier.ObjectMeta.Namespace)
		basic, found := tiers["basic"]
		require.True(t, found)
		assert.Equal(t, "basic", basic.ObjectMeta.Name)
		assert.Equal(t, namespace, basic.ObjectMeta.Namespace)
	})

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
