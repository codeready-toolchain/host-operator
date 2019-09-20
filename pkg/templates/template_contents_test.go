package templates_test

import (
	"fmt"
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/templates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplates(t *testing.T) {

	tiers := []string{"basic", "advanced"}
	nsTypes := []string{"code", "dev", "stage"}

	for _, tier := range tiers {
		for _, nsType := range nsTypes {
			t.Run(fmt.Sprintf("%s-%s", tier, nsType), func(t *testing.T) {
				// when
				asset, err := templates.Asset(fmt.Sprintf("%s-%s.yml", tier, nsType))
				// then
				require.NoError(t, err)
				assert.NotEmpty(t, asset)
			})
		}
	}
}
