package version_test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/version"

	"github.com/stretchr/testify/assert"
)

func TestVars(t *testing.T) {
	// simply verify that the vars exist.
	// They will be populated by the `-ldflags="-X ..."` at build time
	assert.Equal(t, "unknown", version.Commit)
	assert.Equal(t, "unknown", version.BuildTime)
}
