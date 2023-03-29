package segment

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/segment"
	"github.com/segmentio/analytics-go/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func AssertMessageQueued(t *testing.T, cl *segment.Client, us *toolchainv1alpha1.UserSignup, event string) {
	require.IsType(t, &MockClient{}, cl.Client())
	require.Len(t, cl.Client().(*MockClient).Queue, 1)
	assert.Equal(t, analytics.Track{
		UserId:     segment.Hash(us.Spec.Username),
		Event:      event,
		Properties: analytics.NewProperties().Set("user_id", us.Spec.Userid),
		Context: &analytics.Context{
			Extra: map[string]interface{}{
				"account_id": us.Annotations[toolchainv1alpha1.SSOAccountIDAnnotationKey],
			},
		},
	}, cl.Client().(*MockClient).Queue[0])
}

func AssertNoMessageQueued(t *testing.T, cl *segment.Client) {
	require.IsType(t, &MockClient{}, cl.Client())
	assert.Empty(t, cl.Client().(*MockClient).Queue, 0)
}
