package segment

import (
	"github.com/segmentio/analytics-go/v3"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Client struct {
	client analytics.Client
}

var logger = logf.Log.WithName("segment_client")

func NewClient(key string) (*Client, error) {
	cl, err := analytics.NewWithConfig(key,
		analytics.Config{
			// Logger: nil,
		})
	if err != nil {
		return nil, err
	}
	return &Client{client: cl}, err
}

func (c *Client) Close() error {
	return c.client.Close()
}

const accountActivated = "account activated"

func (c *Client) TrackAccountActivation(username string) error {
	logger.Info("sending event to Segment", "event", accountActivated, "username_hash", hash(username))
	return c.client.Enqueue(analytics.Track{
		UserId: hash(username),
		Event:  accountActivated,
	})
}
