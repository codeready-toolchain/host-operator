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
	assertMessageQueued(t, cl, murName, us.Spec.IdentityClaims.UserID, us.Spec.IdentityClaims.AccountID, "account activated")
}

func assertMessageQueued(t *testing.T, cl *segment.Client, username, userID, accountID string, event string) {
	require.IsType(t, &MockClient{}, cl.Client())
	require.Len(t, cl.Client().(*MockClient).Queue, 1)
	expectedTrackEvent := analytics.Track{
		UserId: segment.Hash(username),
		Event:  event,
		Properties: analytics.Properties{
			"user_id":    userID,
			"account_id": accountID,
		},
		Context: &analytics.Context{
			Extra: map[string]interface{}{
				"groupId": accountID,
			},
		},
	}
	actualMessage := cl.Client().(*MockClient).Queue[0]
	actualTrackEvent, ok := actualMessage.(analytics.Track)
	require.True(t, ok)
	assert.Equal(t, expectedTrackEvent.UserId, actualTrackEvent.UserId)
	assert.Equal(t, expectedTrackEvent.Event, actualTrackEvent.Event)
	assert.Equal(t, expectedTrackEvent.Context, actualTrackEvent.Context)
	assert.Equal(t, expectedTrackEvent.Properties["user_id"], actualTrackEvent.Properties["user_id"])
	assert.Equal(t, expectedTrackEvent.Properties["account_id"], actualTrackEvent.Properties["account_id"])
	assert.NotEmpty(t, actualTrackEvent.Properties["epoch_time"]) // not comparing the timestamp in test
}

func AssertNoMessageQueued(t *testing.T, cl *segment.Client) {
	require.IsType(t, &MockClient{}, cl.Client())
	assert.Empty(t, cl.Client().(*MockClient).Queue, 0)
}
