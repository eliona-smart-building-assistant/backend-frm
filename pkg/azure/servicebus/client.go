package servicebus

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

type Client struct {
	client *azservicebus.Client
}

func NewClient(connStr string) (*Client, error) {
	sbClient, err := azservicebus.NewClientFromConnectionString(connStr, nil)
	if err != nil {
		return nil, err
	}

	return &Client{sbClient}, nil
}

func (c *Client) Close(ctx context.Context) error {
	return c.client.Close(ctx)
}

func (c *Client) NewQueueListener(queue string) (*azservicebus.Receiver, error) {
	return c.client.NewReceiverForQueue(queue, nil)
}
