package kafka

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/eliona-smart-building-assistant/backend-frm/pkg/log"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Record = *kgo.Record

func NewRecord() Record {
	return &kgo.Record{}
}

type HandlerFunc func(Record)

type Subscriptions map[string]HandlerFunc

type eventCallbacks struct {
	onPartitionsAssigned []func(context.Context, *kgo.Client, map[string][]int32)
	onPartitionsRevoked  []func(context.Context, *kgo.Client, map[string][]int32)
	onPartitionsLost     []func(context.Context, *kgo.Client, map[string][]int32)
	onConsumerError      func(error)
	onPublishResult      func(Record)
}

type config struct {
	splitConsumer  bool
	manualCommit   bool
	blockRebalance bool
	maxFetches     int
}

type consumer struct {
	sc      *splitConsumer
	done    chan struct{}
	running bool
}

type Client struct {
	client        *kgo.Client
	pingTimeout   time.Duration
	pingRetries   int
	pingBackoff   time.Duration
	logger        log.Logger
	opts          []kgo.Opt
	subsMu        sync.Mutex
	subscriptions Subscriptions
	shutdown      chan struct{}
	stopConsuming chan struct{}
	commitQueue   chan *kgo.Record
	wg            sync.WaitGroup
	callbacks     eventCallbacks
	config        config
	consumer      consumer
}

func defaultClient() *Client {
	hostname, _ := os.Hostname()
	return &Client{
		callbacks: eventCallbacks{
			onConsumerError: func(error) {},
			onPublishResult: func(Record) {},
		},
		config: config{
			maxFetches: 1,
		},
		pingTimeout: 10 * time.Second,
		pingRetries: 100,
		pingBackoff: 3 * time.Second,
		logger:      log.NoopLogger(),
		opts:        []kgo.Opt{kgo.ClientID(hostname)},
		shutdown:    make(chan struct{}),
	}
}

func New(opts ...Opt) (*Client, error) {
	var err error
	client := defaultClient()

	for _, opt := range opts {
		opt(client)
	}

	if client.config.splitConsumer {
		client.consumer.sc = newSplitConsumer(client)
		client.callbacks.onPartitionsAssigned = append(client.callbacks.onPartitionsAssigned, client.consumer.sc.onPartitionsAssigned)
		client.callbacks.onPartitionsRevoked = append(client.callbacks.onPartitionsRevoked, client.consumer.sc.onPartitionsLost)
		client.callbacks.onPartitionsLost = append(client.callbacks.onPartitionsLost, client.consumer.sc.onPartitionsLost)
	}

	client.assignPartitionCallbacks()

	client.client, err = kgo.NewClient(client.opts...)
	if err != nil {
		return nil, err
	}

	logger := client.logger.With().
		Str("module", "kafka").
		Str("client_id", client.client.OptValue(kgo.ClientID).(string)).
		Logger()
	client.logger = &logger

	if err = client.pingWithRetry(context.Background()); err != nil {
		return nil, err
	}

	if client.config.manualCommit {
		client.wg.Add(1)
		go client.commitWorker()
	}

	for topic := range client.subscriptions {
		client.client.AddConsumeTopics(topic)
	}

	client.logger.Info().Msg("client initialized")

	return client, nil
}

func (c *Client) assignPartitionCallbacks() {
	if len(c.callbacks.onPartitionsAssigned) > 0 {
		c.opts = append(c.opts, kgo.OnPartitionsAssigned(func(ctx context.Context, cl *kgo.Client, m map[string][]int32) {
			for _, fn := range c.callbacks.onPartitionsAssigned {
				fn(ctx, cl, m)
			}
		}))
	}

	if len(c.callbacks.onPartitionsRevoked) > 0 {
		c.opts = append(c.opts, kgo.OnPartitionsRevoked(func(ctx context.Context, cl *kgo.Client, m map[string][]int32) {
			for _, fn := range c.callbacks.onPartitionsRevoked {
				fn(ctx, cl, m)
			}
		}))
	}

	if len(c.callbacks.onPartitionsLost) > 0 {
		c.opts = append(c.opts, kgo.OnPartitionsLost(func(ctx context.Context, cl *kgo.Client, m map[string][]int32) {
			for _, fn := range c.callbacks.onPartitionsLost {
				fn(ctx, cl, m)
			}
		}))
	}
}

func (c *Client) Close() {
	close(c.shutdown)
	c.wg.Wait()
	c.client.CloseAllowingRebalance()
}

// pingWithRetry retries the initial broker ping until it succeeds, pingRetries is
// exhausted, or ctx is cancelled. A broker can be temporarily unreachable at startup
// (DNS not yet resolvable, broker not yet ready) or be rescheduled after the client
// starts, so NewClient must not give up on the first failed ping; the last error is
// returned to the caller once retries are exhausted.
func (c *Client) pingWithRetry(ctx context.Context) error {
	for attempt := 1; ; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, c.pingTimeout)
		err := c.client.Ping(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		if attempt >= c.pingRetries {
			return err
		}
		c.logger.Warn().Err(err).Int("attempt", attempt).Int("max_attempts", c.pingRetries).
			Msg("kafka broker not reachable; retrying")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.pingBackoff):
		}
	}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx)
}

func (c *Client) CommitRecords(r ...Record) {
	for i := range r {
		c.commitQueue <- r[i]
	}
}
