package kafka

import (
	"context"
	"time"

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
		c.config.maxFetches = max
	}
}

func WithManualCommit() func(c *Client) {
	return func(c *Client) {
		c.opts = append(c.opts, kgo.DisableAutoCommit())
		c.config.manualCommit = true
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
		c.callbacks.onConsumerError = fn
	}
}

func WithLogger(l log.Logger) Opt {
	return func(c *Client) {
		c.logger = l
	}
}

type PartitionEvent int

const (
	PartitionAssigned PartitionEvent = iota
	Partitionrevoked
	PartitionLost
)

type PartitionEventCb func(event PartitionEvent, topic string, partition int)

func WithOnPartitionEvents(fn PartitionEventCb) Opt {
	cb := func(event PartitionEvent) func(context.Context, *kgo.Client, map[string][]int32) {
		return func(_ context.Context, _ *kgo.Client, m map[string][]int32) {
			for topic, partitions := range m {
				for i := range partitions {
					fn(event, topic, int(partitions[i]))
				}
			}
		}
	}

	return func(c *Client) {
		c.callbacks.onPartitionsAssigned = append(c.callbacks.onPartitionsAssigned, cb(PartitionAssigned))
		c.callbacks.onPartitionsRevoked = append(c.callbacks.onPartitionsRevoked, cb(Partitionrevoked))
		c.callbacks.onPartitionsLost = append(c.callbacks.onPartitionsLost, cb(PartitionLost))
	}
}

func WithConcurentConsumer() Opt {
	return func(c *Client) {
		c.config.splitConsumer = true
	}
}

func WithBlockRebalanceOnPoll() Opt {
	return func(c *Client) {
		c.opts = append(c.opts, kgo.BlockRebalanceOnPoll())
		c.config.blockRebalance = true
	}
}

func WithPingTimeout(t time.Duration) Opt {
	return func(c *Client) {
		c.pingTimeout = t
	}
}

// WithPingRetries sets how many times the startup broker ping is attempted before
// NewClient gives up (default 100). Each attempt uses the ping timeout.
func WithPingRetries(n int) Opt {
	return func(c *Client) {
		c.pingRetries = n
	}
}

// WithPingBackoff sets the delay between startup broker ping attempts (default 3s).
func WithPingBackoff(d time.Duration) Opt {
	return func(c *Client) {
		c.pingBackoff = d
	}
}
