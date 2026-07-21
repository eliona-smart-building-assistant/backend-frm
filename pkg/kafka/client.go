package kafka

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/eliona-smart-building-assistant/backend-frm/pkg/log"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	defaultPingTimeout = 10 * time.Second
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
	maxFetches     int
	pingTimeout    time.Duration
	splitConsumer  bool
	manualCommit   bool
	blockRebalance bool
	noInitPing     bool
}

type consumer struct {
	sc      *splitConsumer
	done    chan struct{}
	running bool
}

type Client struct {
	client        *kgo.Client
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
			maxFetches:  1,
			pingTimeout: defaultPingTimeout,
		},
		logger:   log.NoopLogger(),
		opts:     []kgo.Opt{kgo.ClientID(hostname)},
		shutdown: make(chan struct{}),
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

	if !client.config.noInitPing {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), client.config.pingTimeout)
		defer pingCancel()
		err = client.client.Ping(pingCtx)
		if err != nil {
			return nil, err
		}
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

// Ping tests the connection to seed brokers using ctx as cancelation/timeout
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx)
}

func (c *Client) CommitRecords(r ...Record) {
	for i := range r {
		c.commitQueue <- r[i]
	}
}

func (c *Client) Flush(ctx context.Context) error {
	return c.client.Flush(ctx)
}
