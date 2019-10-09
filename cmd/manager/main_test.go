package main_test

import (
	"testing"

	main "github.com/codeready-toolchain/host-operator/cmd/manager"

	"github.com/stretchr/testify/assert"
)

func TestVars(t *testing.T) {
	// simply verify that the vars exist.
	// They will be populated by the `-ldflags="-X ..."` at build time
	assert.Equal(t, "unknown", main.Commit)
	assert.Equal(t, "unknown", main.BuildTime)
}
