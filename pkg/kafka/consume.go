package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	commitInterval = 100 * time.Millisecond
)

// AddConsumeTopic add a specified topic to an internal consumption list.
//
// If a client is initialized with the WithSubscriptions option, then this call is no-op - use AddSubscription instead
func (c *Client) AddConsumeTopic(topic string) {
	if c.subscriptions != nil {
		return
	}

	c.client.AddConsumeTopics(topic)
}

func (c *Client) RemoveConsumeTopic(topic string) {
	if c.subscriptions != nil {
		return
	}

	c.client.PurgeTopicsFromClient(topic)
}

func (c *Client) AddSubscription(topic string, handler HandlerFunc) {
	c.subsMu.Lock()
	if c.subscriptions == nil {
		c.subscriptions = make(map[string]HandlerFunc)
	}
	c.subscriptions[topic] = handler
	c.client.AddConsumeTopics(topic)
	c.subsMu.Unlock()
}

func (c *Client) RemoveSubscription(topic string) {
	c.subsMu.Lock()
	delete(c.subscriptions, topic)
	c.client.PurgeTopicsFromClient(topic)
	c.subsMu.Unlock()
}

// StartConsumer starts background polling of records on topics defined in Subscriptions.
// Consumption is stopped either by canceling the context or via [Client.StopConsumer]
func (c *Client) StartConsumer(ctx context.Context) {
	c.stopConsuming = make(chan struct{})
	c.consumer.running = true

	c.wg.Add(1)
	if c.config.splitConsumer {
		go c.consumer.sc.consume(ctx)
	} else {
		go c.consumeWorker(ctx)
	}
}

// StopConsumer stops the background consuming of records. If the fetches are already being processed,
// it will block untill processing is finished.
//
// This function can also be used to check wether the consumer has stopped after the cancelation of context
// passe to [Client.StartConsumer]
func (c *Client) StopConsumer() {
	if !c.consumer.running {
		return
	}

	close(c.stopConsuming)
	c.logger.Debug().Msg("waiting for consumer to finish processing")
	<-c.consumer.done
}

func (c *Client) consumeWorker(ctx context.Context) {
	c.consumer.done = make(chan struct{})

	defer func() {
		close(c.consumer.done)
		c.wg.Done()
		c.consumer.running = false
	}()

	maxFetches := c.config.maxFetches
	for {
		select {
		case <-c.shutdown:
			return
		case <-c.stopConsuming:
			return
		default:
			fetches := c.client.PollRecords(ctx, maxFetches)
			if fetches.IsClientClosed() {
				return
			}

			if errs := fetches.Errors(); len(errs) > 0 {
				for i := range errs {
					c.callbacks.onConsumerError(wrapKgoConsumerError(errs[i].Err))
				}

				continue
			}

			c.subsMu.Lock()
			fetches.EachRecord(func(r *kgo.Record) {
				c.subscriptions[r.Topic](r)
			})
			c.subsMu.Unlock()
		}
	}
}

func (c *Client) commitWorker() {
	defer c.wg.Done()

	maxFetches := c.config.maxFetches
	pendingCommits := make([]*kgo.Record, 0, maxFetches)
	ticker := time.NewTicker(commitInterval)
	defer ticker.Stop()

	for {
		select {
		case r := <-c.commitQueue:
			pendingCommits = append(pendingCommits, r)
			if len(pendingCommits) >= maxFetches {
				_ = c.client.CommitRecords(c.client.Context(), pendingCommits...)
				clear(pendingCommits)
				pendingCommits = pendingCommits[:0]
			}

		case <-ticker.C:
			if len(pendingCommits) > 0 {
				_ = c.client.CommitRecords(c.client.Context(), pendingCommits...)
				clear(pendingCommits)
				pendingCommits = pendingCommits[:0]
			}

		case <-c.shutdown:
			if len(pendingCommits) > 0 {
				_ = c.client.CommitRecords(c.client.Context(), pendingCommits...)
				clear(pendingCommits)
				pendingCommits = pendingCommits[:0]
			}
			return
		}
	}
}

// EachNewRecord fetches up to [WithMaxFetchCount] records and calls fn for each of them
func (c *Client) EachNewRecord(ctx context.Context, fn func(r Record)) error {
	if c.consumer.running {
		return fmt.Errorf("background consumer running")
	}

	fetches := c.client.PollRecords(ctx, c.config.maxFetches)
	if fetches.IsClientClosed() {
		return kgo.ErrClientClosed
	}

	if errs := fetches.Errors(); len(errs) > 0 {
		return fetches.Err0()
	}

	fetches.EachRecord(func(record *kgo.Record) {
		fn(record)
	})

	return nil
}

// FetchRecordsBatch return a slice of [Record] which max length is set with [WithMaxFetchCount]
func (c *Client) FetchRecordsBatch(ctx context.Context) ([]Record, error) {
	if c.consumer.running {
		return nil, fmt.Errorf("background consumer running")
	}

	fetches := c.client.PollRecords(ctx, c.config.maxFetches)
	if fetches.IsClientClosed() {
		return nil, kgo.ErrClientClosed
	}

	if errs := fetches.Errors(); len(errs) > 0 {
		return nil, fetches.Err0()
	}

	return fetches.Records(), nil
}
