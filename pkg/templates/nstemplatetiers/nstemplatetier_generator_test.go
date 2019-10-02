package nstemplatetiers_test

import (
	"bytes"
	"testing"
	texttemplate "text/template"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"
	uuid "github.com/satori/go.uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestNewNSTemplateTier(t *testing.T) {

	// uses the `Asset` func generated in `pkg/templates/template_contents_test.go` here
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	g, err := nstemplatetiers.NewNSTemplateTierGenerator(s, testnstemplatetiers.Asset)
	require.NoError(t, err)
	namespace := "host-operator" + uuid.NewV4().String()[:7]

	t.Run("ok", func(t *testing.T) {
		// given
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
				actual, err := g.NewNSTemplateTier(tier, namespace)
				// then
				require.NoError(t, err)
				expected, expectedStr, err := newNSTemplateTierFromYAML(s, tier, namespace, revisions)
				require.NoError(t, err)
				t.Logf("expected NSTemplateTier (yaml):\n%s", expectedStr)
				// here we don't compare whoe objects because the generated NSTemplateTier
				// has no specific values for the `TypeMeta`: the `APIVersion: toolchain.dev.openshift.com/v1alpha1`
				// and `Kind: NSTemplateTier` should be set by the client using the registered GVK
				assert.Equal(t, expected.ObjectMeta, actual.ObjectMeta)
				assert.Equal(t, expected.Spec, actual.Spec)
			})
		}
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("unknown tier", func(t *testing.T) {
			// when
			_, err := g.NewNSTemplateTier("foo", namespace)
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
	// uses the `Asset` func generated in `pkg/templates/template_contents_test.go` here
	g, err := nstemplatetiers.NewNSTemplateTierGenerator(s, testnstemplatetiers.Asset)
	require.NoError(t, err)
	namespace := "host-operator" + uuid.NewV4().String()[:7]

	t.Run("ok", func(t *testing.T) {
		// when
		tiers, err := g.NewNSTemplateTiers(namespace)
		// then
		require.NoError(t, err)
		require.Len(t, tiers, 2)
		// sort by name
		assert.Equal(t, "advanced", tiers[0].ObjectMeta.Name)
		assert.Equal(t, namespace, tiers[0].ObjectMeta.Namespace)
		assert.Equal(t, "basic", tiers[1].ObjectMeta.Name)
		assert.Equal(t, namespace, tiers[1].ObjectMeta.Namespace)
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
