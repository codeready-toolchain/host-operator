package nstemplatetiers_test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestNSTemplateTierGenerator(t *testing.T) {

	// uses the `Asset` func generated in `pkg/templates/template_contents_test.go` here
	g, err := nstemplatetiers.NewNSTemplateTierGenerator(testnstemplatetiers.Asset)
	require.NoError(t, err)

	t.Run("basic", func(t *testing.T) {
		// given
		tier := "basic"
		// when
		result, err := g.GenerateManifest(tier)
		// then
		require.NoError(t, err)
		expected := `kind: NSTemplateTier
apiVersion: toolchain.dev.openshift.com/v1alpha1
metadata:
  name: basic
spec:
  namespaces:
  - type: code
    revision: 123456d
    template:
      apiVersion: template.openshift.io/v1
      kind: Template
      metadata:
        labels:
          provider: codeready-toolchain
        name: basic-code
      objects:
      - apiVersion: v1
        kind: Namespace
        metadata:
          annotations:
            openshift.io/description: ${USERNAME}-code
            openshift.io/display-name: ${USERNAME}-code
            openshift.io/requester: ${USERNAME}
          labels:
            provider: codeready-toolchain
          name: ${USERNAME}-code
      parameters:
      - name: USERNAME
        required: true
  - type: dev
    revision: 123456e
    template:
      apiVersion: template.openshift.io/v1
      kind: Template
      metadata:
        labels:
          provider: codeready-toolchain
        name: basic-dev
      objects:
      - apiVersion: v1
        kind: Namespace
        metadata:
          annotations:
            openshift.io/description: ${USERNAME}-dev
            openshift.io/display-name: ${USERNAME}-dev
            openshift.io/requester: ${USERNAME}
          labels:
            provider: codeready-toolchain
          name: ${USERNAME}-dev
      parameters:
      - name: USERNAME
        required: true
  - type: stage
    revision: 123456f
    template:
      apiVersion: template.openshift.io/v1
      kind: Template
      metadata:
        labels:
          provider: codeready-toolchain
        name: basic-stage
      objects:
      - apiVersion: v1
        kind: Namespace
        metadata:
          annotations:
            openshift.io/description: ${USERNAME}-stage
            openshift.io/display-name: ${USERNAME}-stage
            openshift.io/requester: ${USERNAME}
          labels:
            provider: codeready-toolchain
          name: ${USERNAME}-stage
      parameters:
      - name: USERNAME
        required: true`
		assert.Equal(t, expected, string(result))
		// verify that the YAML is valid by converting it to an `NSTemplateTier` object
		err = apis.AddToScheme(scheme.Scheme)
		require.NoError(t, err)
		s := scheme.Scheme
		codecFactory := serializer.NewCodecFactory(s)
		unstructuredTier, _, err := codecFactory.UniversalDeserializer().Decode(result, nil, nil)
		require.NoError(t, err)
		basicTier := toolchainv1alpha1.NSTemplateTier{}
		err = s.Convert(unstructuredTier, &basicTier, nil)
		require.NoError(t, err)
		// quickly verify that the templates in each NSTemplateTier.Spec.Namespaces has some objects
		require.Len(t, basicTier.Spec.Namespaces, 3)
		codeNS := basicTier.Spec.Namespaces[0]
		assert.Len(t, codeNS.Template.Objects, 1)
	})

}
