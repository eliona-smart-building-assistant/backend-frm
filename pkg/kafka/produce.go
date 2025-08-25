package kafka

import "github.com/twmb/franz-go/pkg/kgo"

func (c *Client) Produce(r Record) {
	c.client.Produce(c.client.Context(), r, nil)
}

func (c *Client) ProduceCallback(r Record, callback func(record Record, err error)) {
	c.client.Produce(
		c.client.Context(),
		r,
		func(r *kgo.Record, err error) {
			callback(r, err)
		})
}
