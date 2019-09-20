package templates_test

import (
	"fmt"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/templates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/yaml.v2"
)

func TestTemplates(t *testing.T) {

	tiers := []string{"basic", "advanced"}
	nsTypes := []string{"code", "dev", "stage"}

	for _, tier := range tiers {
		for _, nsType := range nsTypes {
			t.Run(fmt.Sprintf("%s-%s", tier, nsType), func(t *testing.T) {
				// when
				asset, err := templates.Asset(fmt.Sprintf("%s-%s.yaml", tier, nsType))
				// then
				require.NoError(t, err)
				assert.NotEmpty(t, asset)
			})
		}
	}

	t.Run("git commits", func(t *testing.T) {
		// verifies that the metadata file exists and
		// that there is an entry for each namespace above
		// we could check the value, but it's subject to change
		asset, err := templates.Asset("metadata.yaml")
		require.NoError(t, err)
		metadata := make(map[string]interface{})
		err = yaml.Unmarshal(asset, &metadata)
		require.NoError(t, err)
		repo, err := git.PlainOpen("../..")
		require.NoError(t, err)
		ref, err := repo.Head()
		require.NoError(t, err)

		// when/then
		for _, tier := range tiers {
			for _, nsType := range nsTypes {
				filename := fmt.Sprintf("deploy/nstemplatetiers/%s-%s.yaml", tier, nsType)
				commit, found := metadata[fmt.Sprintf("%s-%s", tier, nsType)]
				require.True(t, found)
				assert.Contains(t, metadata, fmt.Sprintf("%s-%s", tier, nsType))

				// verify that the commit in the metadata file matches the
				// actually commit in the repository
				cIter, err := repo.Log(&git.LogOptions{
					From:     ref.Hash(),
					FileName: &filename,
				})
				require.NoError(t, err)
				c, err := cIter.Next()
				require.NoError(t, err)
				assert.Equal(t, c.ID().String()[0:7], commit)
			}
		}
	})
}
