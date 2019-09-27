package templates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRevisions(t *testing.T) {

	// assuming that `metadata.yaml` contains:
	metadata := []byte(`advanced-code: 11111a
advanced-dev: 11111b
advanced-stage: 11111c
basic-code: 22222a
basic-dev: 22222b
basic-stage: 22222c`)

	// when
	revisions, err := parseAllRevisions(metadata)
	// then
	require.NoError(t, err)
	require.Len(t, revisions, 2)
	assert.Equal(t, "11111a", revisions["advanced"]["code"])
	assert.Equal(t, "11111b", revisions["advanced"]["dev"])
	assert.Equal(t, "11111c", revisions["advanced"]["stage"])
	assert.Equal(t, "22222a", revisions["basic"]["code"])
	assert.Equal(t, "22222b", revisions["basic"]["dev"])
	assert.Equal(t, "22222c", revisions["basic"]["stage"])
}

func TestIndent(t *testing.T) {

	// given
	source := []byte(`apiVersion: template.openshift.io/v1
kind: Template
metadata:
  labels:
    provider: codeready-toolchain
  name: advanced-code
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
  required: true`)

	expected := []byte(`  apiVersion: template.openshift.io/v1
  kind: Template
  metadata:
    labels:
      provider: codeready-toolchain
    name: advanced-code
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
    required: true`)

	// when
	actual, err := indent(source, 2)
	// then
	require.NoError(t, err)
	t.Logf("actual:\n%s", string(actual))
	t.Logf("expected:\n%s", string(expected))
	assert.Equal(t, expected, actual)
}
