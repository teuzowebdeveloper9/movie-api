package rabbitmq

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(Exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declaring exchange %s: %w", Exchange, err)
	}
	if err := ch.ExchangeDeclare(DLXExchange, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declaring exchange %s: %w", DLXExchange, err)
	}

	if _, err := ch.QueueDeclare(DLQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declaring queue %s: %w", DLQueue, err)
	}
	if err := ch.QueueBind(DLQueue, "", DLXExchange, false, nil); err != nil {
		return fmt.Errorf("binding queue %s: %w", DLQueue, err)
	}

	if _, err := ch.QueueDeclare(Queue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": DLXExchange,
	}); err != nil {
		return fmt.Errorf("declaring queue %s: %w", Queue, err)
	}
	for _, key := range []string{RoutingKeyCreateRequested, RoutingKeyDeleteRequested} {
		if err := ch.QueueBind(Queue, key, Exchange, false, nil); err != nil {
			return fmt.Errorf("binding queue %s to %s: %w", Queue, key, err)
		}
	}
	return nil
}

func dialWithRetry(ctx context.Context, url string, logger *slog.Logger) (*amqp.Connection, error) {
	backoff := time.Second
	for {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		logger.WarnContext(ctx, "rabbitmq connection failed, retrying", "error", err, "backoff", backoff.String())
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("connecting to rabbitmq: %w", err)
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}
