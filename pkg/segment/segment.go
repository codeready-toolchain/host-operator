package segment

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/segmentio/analytics-go/v3"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Client struct {
	client analytics.Client
}

var logger = logf.Log.WithName("segment_client")

func DefaultClient(key string) (*Client, error) {
	cl, err := analytics.NewWithConfig(key,
		analytics.Config{
			BatchSize: 1,
			Verbose:   true,
			Logger: &loggerAdapter{
				logger: logger,
			},
		})
	if err != nil {
		return nil, err
	}
	return &Client{client: cl}, nil
}

func NewClient(cl analytics.Client) *Client {
	return &Client{client: cl}
}

// Client returns the underlying client.
// For testing purpose only, when the underlying client is a fake impl.
func (c *Client) Client() analytics.Client {
	return c.client
}

const AccountActivated = "account activated"

func (c *Client) TrackAccountActivation(username, userID, accountID string) {
	logger.Info("sending event to Segment", "event", AccountActivated, "username_hash", Hash(username), "userid", userID, "accountid", accountID)
	if err := c.client.Enqueue(analytics.Track{
		Event:  AccountActivated,
		UserId: Hash(username),
		Properties: analytics.NewProperties().
			Set("user_id", userID).
			Set("account_id", accountID).
			Set("epoch_time", time.Now()),
		Context: &analytics.Context{
			Extra: map[string]interface{}{
				"groupId": accountID,
			},
		},
	}); err != nil {
		logger.Error(err, "failed to send event to segment", "action", AccountActivated, "username", username, "userid", userID, "accountid", accountID)
	}
}

func (c *Client) Close() error {
	return c.client.Close()
}

type loggerAdapter struct {
	logger logr.Logger
}

func (l loggerAdapter) Logf(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...)) // using `fmt.Sprintf` to avoid `invalid key: non-string key argument passed to logging, ignoring all later arguments` errors
}

func (l loggerAdapter) Errorf(format string, args ...interface{}) {
	l.logger.Error(nil, fmt.Sprintf(format, args...)) // using `fmt.Sprintf` to avoid `invalid key: non-string key argument passed to logging, ignoring all later arguments` errors
}
