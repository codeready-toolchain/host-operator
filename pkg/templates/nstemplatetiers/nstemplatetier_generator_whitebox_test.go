package nstemplatetiers

import (
	"bytes"
	"fmt"
	"testing"
	texttemplate "text/template"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"
	"github.com/pkg/errors"

	"github.com/codeready-toolchain/host-operator/pkg/apis"
	templatev1 "github.com/openshift/api/template/v1"
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
		g, err := newNSTemplateTierGenerator(s, Asset)
		// then
		require.NoError(t, err)
		// verify that there are tiers with revisions (whatever their values)
		require.NotEmpty(t, g.revisions)
		for tier := range g.revisions {
			assert.NotEmpty(t, g.revisions[tier])
		}
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("asset error", func(t *testing.T) {
			// given
			asset := func(name string) ([]byte, error) {
				return nil, errors.New("test")
			}
			// when initializing the generator with the production Asset
			_, err := newNSTemplateTierGenerator(s, asset)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to initialize the nstemplatetierGenerator: test", err.Error())
		})

		t.Run("unparseable content", func(t *testing.T) {
			// given
			asset := func(name string) ([]byte, error) {
				return []byte("foo::bar"), nil
			}
			// when initializing the generator with the production Asset
			_, err := newNSTemplateTierGenerator(s, asset)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to initialize the nstemplatetierGenerator: unable to parse all template revisions: yaml: unmarshal errors")
		})

		t.Run("unknown metadata.yaml", func(t *testing.T) {
			// given
			asset := func(name string) ([]byte, error) {
				return nil, errors.Errorf("Asset %s not found", name)
			}

			// when
			_, err := newNSTemplateTierGenerator(scheme.Scheme, asset)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to initialize the nstemplatetierGenerator: Asset metadata.yaml not found")
		})
	})
}

func TestParseAllRevisions(t *testing.T) {

	t.Run("ok", func(t *testing.T) {
		// using the `metadata.yaml` test asset
		metadata, err := testnstemplatetiers.Asset("metadata.yaml")
		require.NoError(t, err)
		// when
		revisions, err := parseAllRevisions(metadata)
		// then
		require.NoError(t, err)
		require.Len(t, revisions, 2)
		require.NotContains(t, "foo", revisions) // make sure that the `foo: bar` entry was ignored
		assert.Equal(t, "123456a", revisions["advanced"]["code"])
		assert.Equal(t, "123456b", revisions["advanced"]["dev"])
		assert.Equal(t, "123456c", revisions["advanced"]["stage"])
		assert.Equal(t, "123456d", revisions["basic"]["code"])
		assert.Equal(t, "123456e", revisions["basic"]["dev"])
		assert.Equal(t, "123456f", revisions["basic"]["stage"])
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unparseable content", func(t *testing.T) {
			// given
			asset := []byte("foo::bar")
			// when initializing the generator with the production Asset
			_, err := parseAllRevisions(asset)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to parse all template revisions: yaml: unmarshal errors")
		})

		t.Run("invalid key format", func(t *testing.T) {
			// given
			asset := []byte("foo: bar")
			// when initializing the generator with the production Asset
			_, err := parseAllRevisions(asset)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid namespace template filename. Expected format: '<tier_kind>-<namespace_kind>', got foo")
		})

		t.Run("invalid value format", func(t *testing.T) {
			// given
			asset := []byte("foo-bar: true")
			// when initializing the generator with the production Asset
			_, err := parseAllRevisions(asset)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid namespace template filename revision for 'foo-bar'. Expected a string, got a bool ('true')")
		})

	})

}

func TestNewNSTemplateTier(t *testing.T) {

	// uses the `Asset` func generated in `test/templates/nstemplatetiers/nstemplatetier_assets.go` here
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {

		t.Run("with prod assets", func(t *testing.T) {
			// given
			g, err := newNSTemplateTierGenerator(s, Asset)
			require.NoError(t, err)
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			// when
			tiers, err := g.newNSTemplateTiers(namespace)
			// then
			require.NoError(t, err)
			require.NotEmpty(t, tiers)
			decoder := serializer.NewCodecFactory(s).UniversalDeserializer()
			for _, actual := range tiers {
				tierName := actual.Name
				assert.Equal(t, namespace, actual.Namespace)
				require.Len(t, g.revisions[tierName], len(actual.Spec.Namespaces))
				for nsType, rev := range g.revisions[tierName] {
					found := false
					for _, ns := range actual.Spec.Namespaces {
						if ns.Type == nsType {
							found = true
							assert.Equal(t, rev, ns.Revision)
							asset, err := Asset(fmt.Sprintf("%s-%s.yaml", tierName, nsType))
							require.NoError(t, err)
							tmplObj := &templatev1.Template{}
							_, _, err = decoder.Decode(asset, nil, tmplObj)
							require.NoError(t, err)
							assert.Equal(t, *tmplObj, ns.Template)
							break
						}
					}
					assert.True(t, found, "the namespace with the type wasn't found", "ns-type", nsType)
				}
			}
		})

		t.Run("with test assets", func(t *testing.T) {
			// given
			g, err := newNSTemplateTierGenerator(s, testnstemplatetiers.Asset)
			require.NoError(t, err)
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			data := map[string]map[string]string{
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
			for tier, revisions := range data {
				t.Run(tier, func(t *testing.T) {
					// when
					actual, err := g.newNSTemplateTier(tier, namespace)
					// then
					require.NoError(t, err)
					expected, _, err := newNSTemplateTierFromYAML(s, tier, namespace, revisions)
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
			g, err := newNSTemplateTierGenerator(s, testnstemplatetiers.Asset)
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
		// uses the `Asset` func generated in `test/templates/nstemplatetiers/nstemplatetier_assets.go` here
		g, err := newNSTemplateTierGenerator(s, testnstemplatetiers.Asset)
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

	t.Run("failures", func(t *testing.T) {

		t.Run("unknown asset", func(t *testing.T) {
			// given
			asset := func(name string) ([]byte, error) {
				if name == "metadata.yaml" {
					return []byte("unknown-asset: 123456a"), nil
				}
				return nil, errors.New("foo")
			}
			g, err := newNSTemplateTierGenerator(s, asset)
			require.NoError(t, err)
			namespace := "host-operator" + uuid.NewV4().String()[:7]
			// when
			_, err = g.newNSTemplateTiers(namespace)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to generate all NSTemplateTier manifests: unable to generate 'unknown' NSTemplateTier manifest: foo", err.Error())
		})
	})

}

// newNSTemplateTierFromYAML generates toolchainv1alpha1.NSTemplateTier using a golang template which is applied to the given tier.
func newNSTemplateTierFromYAML(s *runtime.Scheme, tier, namespace string, revisions map[string]string) (toolchainv1alpha1.NSTemplateTier, string, error) {
	expectedTmpl, err := texttemplate.New("template").Parse(`kind: NSTemplateTier
apiVersion: toolchain.dev.openshift.com/v1alpha1
metadata:
  namespace: {{ .Namespace }}
  name: {{ .Tier }}
spec:
  namespaces: 
{{ $tier := .Tier }}{{ range $kind, $revision := .Revisions }}  - type: {{ $kind }}
    revision: {{ $revision }}
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
{{ end }}
      parameters:
      - name: USERNAME
        required: true`)
	if err != nil {
		return toolchainv1alpha1.NSTemplateTier{}, "", err
	}
	expected := bytes.NewBuffer(nil)
	err = expectedTmpl.Execute(expected, struct {
		Tier      string
		Namespace string
		Revisions map[string]string
	}{
		Tier:      tier,
		Namespace: namespace,
		Revisions: revisions,
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
