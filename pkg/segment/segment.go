package segment

import (
	"fmt"

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

func (c *Client) TrackAccountActivation(username string) {
	logger.Info("sending event to Segment", "event", AccountActivated, "username_hash", Hash(username))
	if err := c.client.Enqueue(analytics.Track{
		UserId: Hash(username),
		Event:  AccountActivated,
	}); err != nil {
		logger.Error(err, "failed to send event to segment", "action", AccountActivated, "username", username)
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
