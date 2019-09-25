package templates_test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/templates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestNSTemplateTierGenerator(t *testing.T) {

	t.Run("basic", func(t *testing.T) {
		// given
		tier := "basic"
		// when
		result, err := templates.GenerateNSTemplateTierManifest(tier)
		// then
		require.NoError(t, err)
		expected := `kind: NSTemplateTier
metadata:
  name: basic
spec:
  namespaces:
  - type: code
    revision: 139f5cf
    template: >
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
    revision: 139f5cf
    template: >
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
    revision: 139f5cf
    template: >
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
		// verify that the YAML is valid by parsing it
		yml := make(map[string]interface{})
		err = yaml.Unmarshal(result, &yml)
		require.NoError(t, err)
	})

}
