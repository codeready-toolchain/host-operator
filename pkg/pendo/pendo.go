package pendo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

const AccountActivated = "account activated"
const TrackApiUrl = "https://app.pendo.io/data/track"

type Client struct {
	client        http.Client
	trackEventKey string
}

var logger = logf.Log.WithName("pendo_client")

func DefaultClient(key string) (*Client, error) {
	cl := http.Client{}

	return &Client{client: cl, trackEventKey: key}, nil
}

func (c *Client) TrackAccountActivation(username, userID, accountID string) error {
	// Define payload
	payload := map[string]interface{}{
		"type":      "track",
		"event":     AccountActivated,
		"visitorId": userID,
		"accountId": accountID,
		"timestamp": time.Now,
		"properties": map[string]interface{}{
			"username": username,
		},
	}

	//convert payload to json
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		logger.Error(err, "Unable to marshall payload")
	}

	req, err := http.NewRequest("POST", TrackApiUrl, bytes.NewBuffer(payloadJSON))
	if err != nil {
		logger.Error(err, "Error creating post request")
		return fmt.Errorf("Error creating post request: %s", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-pendo-integration-key", c.trackEventKey)

	res, err := c.client.Do(req)
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("Error sending request: %s", err)
	}
	defer res.Body.Close()

	// Check the response status
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send event to pendo, Error response received: %s", res.Status)
	}

	logger.Info("event successfully sent to Pendo", "event", AccountActivated, "userid", userID, "accountid", accountID)
	return nil
}
