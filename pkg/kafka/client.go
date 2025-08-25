package kafka

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Record *kgo.Record

func NewRecord() Record {
	return &kgo.Record{}
}

type HandlerFunc func(Record)

type Subscriptions map[string]HandlerFunc

type Client struct {
	client          *kgo.Client
	logger          Logger
	manualCommit    bool
	consumerRunning bool
	opts            []kgo.Opt
	subsMu          sync.Mutex
	subscriptions   Subscriptions
	shutdown        chan struct{}
	commitQueue     chan *kgo.Record
	wg              sync.WaitGroup
	maxFetches      int
}

func defaultClient() *Client {
	hostname, _ := os.Hostname()
	return &Client{
		logger:     NoopLogger{},
		opts:       []kgo.Opt{kgo.ClientID(hostname)},
		shutdown:   make(chan struct{}),
		maxFetches: 1,
	}
}

func New(opts ...Opt) (*Client, error) {
	var err error
	client := defaultClient()

	for _, opt := range opts {
		opt(client)
	}

	client.client, err = kgo.NewClient(client.opts...)
	if err != nil {
		return nil, err
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer pingCancel()
	err = client.client.Ping(pingCtx)
	if err != nil {
		return nil, err
	}

	if client.manualCommit {
		client.wg.Add(1)
		go client.commitWorker()
	}

	for topic := range client.subscriptions {
		client.client.AddConsumeTopics(topic)
	}

	return client, nil
}

func (c *Client) Close() {
	close(c.shutdown)
	c.wg.Wait()
	c.client.CloseAllowingRebalance()
}

func (c *Client) CommitRecords(r ...Record) {
	for i := range r {
		c.commitQueue <- r[i]
	}
}
