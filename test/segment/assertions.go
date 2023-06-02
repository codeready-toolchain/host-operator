package segment

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/segment"

	"github.com/segmentio/analytics-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func AssertMessageQueuedForProvisionedMur(t *testing.T, cl *segment.Client, us *toolchainv1alpha1.UserSignup, murName string) {
	assertMessageQueued(t, cl, murName, us.Annotations[toolchainv1alpha1.SSOUserIDAnnotationKey], us.Annotations[toolchainv1alpha1.SSOAccountIDAnnotationKey], "account activated")
}

func assertMessageQueued(t *testing.T, cl *segment.Client, username, userID, accountID string, event string) {
	require.IsType(t, &MockClient{}, cl.Client())
	require.Len(t, cl.Client().(*MockClient).Queue, 1)
	assert.Equal(t, analytics.Track{
		UserId:     segment.Hash(username),
		Event:      event,
		Properties: analytics.NewProperties().Set("user_id", userID),
		Context: &analytics.Context{
			Extra: map[string]interface{}{
				"groupId": accountID,
			},
		},
	}, cl.Client().(*MockClient).Queue[0])
}

func AssertNoMessageQueued(t *testing.T, cl *segment.Client) {
	require.IsType(t, &MockClient{}, cl.Client())
	assert.Empty(t, cl.Client().(*MockClient).Queue, 0)
}
