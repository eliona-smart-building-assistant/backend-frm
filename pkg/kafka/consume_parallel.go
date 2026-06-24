package kafka

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/eliona-smart-building-assistant/backend-frm/pkg/log"
	"github.com/twmb/franz-go/pkg/kgo"
)

type partitionConsumer struct {
	stop    chan struct{}
	records chan []*kgo.Record
}

func (pc *partitionConsumer) consume(logger log.Logger, handler HandlerFunc) {
	logger.Debug().Msg("consumer started")
	defer logger.Debug().Msg("consumer closed")

	for {
		select {
		case <-pc.stop:
			return
		case records := <-pc.records:
			for _, record := range records {
				handler(record)
			}
		}
	}
}

type splitConsumer struct {
	mu        sync.Mutex
	client    *Client
	consumers map[string]map[int32]partitionConsumer
}

func newSplitConsumer(c *Client) *splitConsumer {
	return &splitConsumer{
		client:    c,
		consumers: make(map[string]map[int32]partitionConsumer),
	}
}

func (s *splitConsumer) onPartitionsAssigned(_ context.Context, _ *kgo.Client, assigned map[string][]int32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for topic, partitions := range assigned {
		s.client.logger.Debug().
			Str("topic", topic).
			Ints32("partitions", partitions).
			Msg("partitions assigned")

		if s.consumers[topic] == nil {
			s.consumers[topic] = make(map[int32]partitionConsumer)
		}

		for _, partition := range partitions {
			pc := partitionConsumer{
				stop:    make(chan struct{}),
				records: make(chan []*kgo.Record, 100),
			}
			s.consumers[topic][partition] = pc
			consumerLogger := s.client.logger.With().Str("topic", topic).Int32("partition", partition).Logger()
			go pc.consume(&consumerLogger, s.client.subscriptions[topic])
		}
	}
}

func (s *splitConsumer) onPartitionsLost(_ context.Context, _ *kgo.Client, lost map[string][]int32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for topic, partitions := range lost {
		consumers := s.consumers[topic]
		for _, partition := range partitions {
			consumer := consumers[partition]
			delete(consumers, partition)
			if len(consumers) == 0 {
				delete(s.consumers, topic)
			}
			close(consumer.stop)
		}
	}
}

func (s *splitConsumer) consume(ctx context.Context) {
	s.client.consumer.done = make(chan struct{})

	defer func() {
		close(s.client.consumer.done)
		s.client.wg.Done()
		s.client.consumer.running = false
		if s.client.config.blockRebalance {
			s.client.AllowRebalance()
		}
	}()

	cl := s.client.client
	maxFetches := s.client.config.maxFetches
	blockRebalance := s.client.config.blockRebalance

	for {
		select {
		case <-s.client.shutdown:
			return
		case <-s.client.stopConsuming:
			return
		default:
		}

		fetches := s.poll(ctx, cl, blockRebalance, maxFetches)
		if fetches.IsClientClosed() {
			return
		}

		if errors.Is(fetches.Err0(), context.DeadlineExceeded) {
			s.client.logger.Debug().Err(fetches.Err0()).Msg("polling timeout, next loop")
			if s.client.config.blockRebalance {
				s.client.AllowRebalance()
			}
			continue
		}

		fetches.EachError(func(t string, p int32, err error) {
			s.client.callbacks.onConsumerError(err)
		})

		fetches.EachTopic(func(t kgo.FetchTopic) {
			s.mu.Lock()
			consumers := s.consumers[t.Topic]
			s.mu.Unlock()
			if consumers == nil {
				return
			}

			t.EachPartition(func(p kgo.FetchPartition) {
				consumer, ok := consumers[p.Partition]
				if !ok {
					return
				}

				select {
				case consumer.records <- p.Records:
				case <-consumer.stop:
				}
			})
		})

		if blockRebalance {
			cl.AllowRebalance()
		}
	}
}

func (s *splitConsumer) poll(ctx context.Context, cl *kgo.Client, blockRebalance bool, maxFetches int) kgo.Fetches {
	var cancel context.CancelFunc
	if blockRebalance {
		ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
	}

	return cl.PollRecords(ctx, maxFetches)
}
