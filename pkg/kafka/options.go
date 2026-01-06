package kafka

import (
	"context"

	"github.com/eliona-smart-building-assistant/backend-frm/pkg/log"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Opt func(*Client)

func Seeds(seeds ...string) func(*Client) {
	return func(c *Client) {
		c.opts = append(c.opts, kgo.SeedBrokers(seeds...))
	}
}

// WithSubscriptions subscribes to topics that are map keys.
//
// Use the AddConsumeTopic method of Client to add topic to consume for manual consumption
func WithSubscriptions(s Subscriptions) func(*Client) {
	return func(c *Client) {
		c.subscriptions = s
	}
}

func WithClientID(id string) func(*Client) {
	return func(c *Client) {
		c.opts = append(c.opts, kgo.ClientID(id))
	}
}

func WithGroup(group string) func(*Client) {
	return func(c *Client) {
		c.opts = append(c.opts, kgo.ConsumerGroup(group))
	}
}

func WithMaxFetchCount(max int) func(*Client) {
	return func(c *Client) {
		c.maxFetches = max
	}
}

func WithManualCommit() func(c *Client) {
	return func(c *Client) {
		c.opts = append(c.opts, kgo.DisableAutoCommit())
		c.manualCommit = true
		c.commitQueue = make(chan *kgo.Record, 1)
	}
}

func WithContext(ctx context.Context) Opt {
	return func(c *Client) {
		c.opts = append(c.opts, kgo.WithContext(ctx))
	}
}

func ResetOffsetsToEnd() Opt {
	return func(c *Client) {
		c.opts = append(c.opts, kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()))
	}
}

func WithOnError(fn func(error)) Opt {
	return func(c *Client) {
		c.onError = fn
	}
}

func WithLogger(l log.Logger) Opt {
	return func(c *Client) {
		c.logger = l
	}
}
