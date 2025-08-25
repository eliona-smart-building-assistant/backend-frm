package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler func(channel string, payload string)

type Listener struct {
	ReconnectDelay time.Duration
	db             *pgxpool.Pool
	handlers       map[string]Handler
}

func (p *Pool) NewListener() *Listener {
	return &Listener{
		db: p.pool,
	}
}

func (l *Listener) Handle(channel string, handler Handler) {
	if l.handlers == nil {
		l.handlers = make(map[string]Handler)
	}

	l.handlers[channel] = handler
}

func (l *Listener) Listen(ctx context.Context, channel string, handler Handler) error {
	if l.handlers == nil {
		return errors.New("listener: no handlers defined")
	}

	reconnectDelay := 10 * time.Second
	if l.ReconnectDelay != 0 {
		reconnectDelay = l.ReconnectDelay
	}

	for {
		err := l.listen(ctx)
		if err != nil {
			// TODO: send error to err chan
		}

		if reconnectDelay < 0 {
			if err := ctx.Err(); err != nil {
				return err
			}

			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(reconnectDelay):

		}
	}
}

func (l *Listener) listen(ctx context.Context) error {
	conn, err := l.db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("listener: connect: %w", err)
	}

	defer func() {
		_ = conn.Conn().Close(ctx)
		conn.Release()
	}()

	for channel := range l.handlers {
		_, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize())
		if err != nil {
			return fmt.Errorf("listener: listen %q: %w", channel, err)
		}
	}

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("listener: wait for notification: %w", err)
		}

		if handler, ok := l.handlers[notification.Channel]; ok {
			handler(notification.Channel, notification.Payload)
		}
	}
}
