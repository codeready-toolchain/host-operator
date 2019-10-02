package nstemplatetiers

import (
	"errors"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/apis"
	testnstemplatetiers "github.com/codeready-toolchain/host-operator/test/templates/nstemplatetiers"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestNewNSTemplateTierGenerator(t *testing.T) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		// when initializing the generator with the production Asset
		g, err := NewNSTemplateTierGenerator(s, Asset)
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
			_, err := NewNSTemplateTierGenerator(s, asset)
			// then
			require.Error(t, err)
			assert.Equal(t, "unable to initialize the NSTemplateTierGenerator: test", err.Error())
		})

		t.Run("unparseable content", func(t *testing.T) {
			// given
			asset := func(name string) ([]byte, error) {
				return []byte("foo::bar"), nil
			}
			// when initializing the generator with the production Asset
			_, err := NewNSTemplateTierGenerator(s, asset)
			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to initialize the NSTemplateTierGenerator: unable to parse all template revisions: yaml: unmarshal errors")
		})
	})
}
