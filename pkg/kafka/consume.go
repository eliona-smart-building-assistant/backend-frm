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

// AddConsumeTopic add specified topic to internal consumption list.
//
// If client is initialized with WithSubscriptions option then this call is no-op - use AddSubscription instead
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

// StartConsumer starts background polling of records on topics defined in Subscriptions
func (c *Client) StartConsumer(ctx context.Context) {
	c.consumerRunning = true
	c.wg.Add(1)
	go c.consumeWorker(ctx)
}

func (c *Client) consumeWorker(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-c.shutdown:
			return
		default:
			fetches := c.client.PollRecords(ctx, c.maxFetches)
			if fetches.IsClientClosed() {
				return
			}

			if errs := fetches.Errors(); len(errs) > 0 {
				for i := range errs {
					c.logger.Error("Kafka", "Error polling records: %v", errs[i])
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

	pendingCommits := make([]*kgo.Record, 0, c.maxFetches)
	ticker := time.NewTicker(commitInterval)
	defer ticker.Stop()

	for {
		select {
		case r := <-c.commitQueue:
			pendingCommits = append(pendingCommits, r)
			if len(pendingCommits) >= c.maxFetches {
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

func (c *Client) PollRecords(fn func(r Record)) error {
	if c.consumerRunning {
		return fmt.Errorf("background consumer running")
	}

	fetches := c.client.PollRecords(nil, c.maxFetches)
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

func (c *Client) PollRecordsContext(ctx context.Context, fn func(r Record)) error {
	if c.consumerRunning {
		return fmt.Errorf("background consumer running")
	}

	fetches := c.client.PollRecords(ctx, c.maxFetches)
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

func (c *Client) FetchRecords() ([]*kgo.Record, error) {
	if c.consumerRunning {
		return nil, fmt.Errorf("background consumer running")
	}

	fetches := c.client.PollRecords(nil, c.maxFetches)
	if fetches.IsClientClosed() {
		return nil, kgo.ErrClientClosed
	}

	if errs := fetches.Errors(); len(errs) > 0 {
		return nil, fetches.Err0()
	}

	return fetches.Records(), nil
}

func (c *Client) FetchRecordsContext(ctx context.Context) ([]*kgo.Record, error) {
	if c.consumerRunning {
		return nil, fmt.Errorf("background consumer running")
	}

	fetches := c.client.PollRecords(ctx, c.maxFetches)
	if fetches.IsClientClosed() {
		return nil, kgo.ErrClientClosed
	}

	if errs := fetches.Errors(); len(errs) > 0 {
		return nil, fetches.Err0()
	}

	return fetches.Records(), nil
}
