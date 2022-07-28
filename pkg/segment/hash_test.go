package segment_test

import (
	"testing"

	"github.com/codeready-toolchain/host-operator/pkg/segment"
	"github.com/stretchr/testify/assert"
)

func TestHashUsername(t *testing.T) {

	// given
	username := "test-username"
	// when
	result := segment.Hash(username)
	// then
	assert.Equal(t, "ded736438647e25487ef37c6c6ca2420ef0072db", result)
}
