package segment

import "github.com/segmentio/analytics-go/v3"

func NewClient() analytics.Client {
	return &MockClient{
		Queue: []analytics.Message{},
	}
}

type MockClient struct {
	Queue []analytics.Message
}

var _ analytics.Client = &MockClient{}

func (c *MockClient) Enqueue(m analytics.Message) error {
	c.Queue = append(c.Queue, m)
	return nil
}

func (c *MockClient) Close() error {
	return nil
}
