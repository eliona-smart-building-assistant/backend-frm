package kafka

import (
	"context"
	"os"
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
	hostname, _ := os.Hostname()

	return func(c *Client) {
		c.clientId = id
		c.opts = append(c.opts, kgo.ClientID(id+"-"+hostname))
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
type PartitionEventBulkCb func(event PartitionEvent, topic string, partitions []int)

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

func WithOnPartitionEventsBulk(fn PartitionEventBulkCb) Opt {
	cb := func(event PartitionEvent) func(context.Context, *kgo.Client, map[string][]int32) {
		return func(_ context.Context, _ *kgo.Client, m map[string][]int32) {
			for topic, partitions := range m {
				p := make([]int, len(partitions))
				for i := range partitions {
					p[i] = int(partitions[i])
				}
				fn(event, topic, p)
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

// WithPingTimeout sets initial ping-check (during client creation) timeout to t overriding the default of 10 seconds.
// This option make no sense if [WithNoInitialPing] is used.
func WithPingTimeout(t time.Duration) Opt {
	return func(c *Client) {
		c.config.pingTimeout = t
	}
}

// WithNoInitialPing disables ping-check during client creation.
// You can later check the connectivity via [Client.Ping].
//
// Might be useful when you want to retry the initial connection to broker.
func WithNoInitialPing() Opt {
	return func(c *Client) {
		c.config.noInitPing = true
	}
}
