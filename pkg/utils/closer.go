package utils

import (
	"context"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Closer struct {
	sync.Mutex
	closeTimeout time.Duration
	funcs        []func(ctx context.Context) error
}

func NewCloser(closeTimeout time.Duration) *Closer {
	return &Closer{closeTimeout: closeTimeout}
}

func (c *Closer) Add(f func(ctx context.Context) error) {
	c.Lock()
	defer c.Unlock()

	c.funcs = append(c.funcs, f)
}

func (c *Closer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.closeTimeout)
	defer cancel()

	doneCh := make(chan struct{})
	go func() {
		wg := sync.WaitGroup{}
		wg.Add(len(c.funcs))

		for _, f := range c.funcs {
			go func() {
				defer wg.Done()

				if err := f(ctx); err != nil {
					// TODO: log the error
				}
			}()
		}

		wg.Wait()
		doneCh <- struct{}{}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-doneCh:
		return nil
	}
}

func (c *Closer) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	<-ctx.Done()
	return c.Close()
}
