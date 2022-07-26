package segment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashUsername(t *testing.T) {

	// given
	username := "test-username"
	// when
	result := hash(username)
	// then
	assert.Equal(t, "ded736438647e25487ef37c6c6ca2420ef0072db", result)
}
