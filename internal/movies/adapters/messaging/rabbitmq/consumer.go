package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

type Consumer struct {
	url     string
	applier ports.MovieEventApplier
	logger  *slog.Logger
}

func NewConsumer(url string, applier ports.MovieEventApplier, logger *slog.Logger) *Consumer {
	return &Consumer{url: url, applier: applier, logger: logger}
}

func (c *Consumer) Run(ctx context.Context) error {
	for {
		err := c.consume(ctx)
		if ctx.Err() != nil {
			return nil
		}
		c.logger.ErrorContext(ctx, "rabbitmq consumer disconnected, reconnecting", "error", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
}

func (c *Consumer) consume(ctx context.Context) error {
	conn, err := dialWithRetry(ctx, c.url, c.logger)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("opening channel: %w", err)
	}
	if err := declareTopology(ch); err != nil {
		return err
	}

	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("setting qos: %w", err)
	}

	deliveries, err := ch.Consume(Queue, "movies-service", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("starting consumer: %w", err)
	}
	c.logger.InfoContext(ctx, "rabbitmq consumer started", "queue", Queue)

	closed := conn.NotifyClose(make(chan *amqp.Error, 1))
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-closed:
			return fmt.Errorf("connection closed: %w", err)
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("delivery channel closed")
			}
			c.handle(ctx, delivery)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, delivery amqp.Delivery) {
	var env envelope
	if err := json.Unmarshal(delivery.Body, &env); err != nil {
		c.logger.ErrorContext(ctx, "malformed event, sending to DLQ", "error", err)
		c.reject(ctx, delivery, false)
		return
	}

	logger := c.logger.With("event_id", env.EventID, "type", env.Type)
	err := c.apply(ctx, env)
	switch {
	case err == nil:
		if ackErr := delivery.Ack(false); ackErr != nil {
			logger.ErrorContext(ctx, "failed to ack event", "error", ackErr)
		}
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, errUnknownEvent):
		logger.ErrorContext(ctx, "poison event, sending to DLQ", "error", err)
		c.reject(ctx, delivery, false)
	default:
		logger.WarnContext(ctx, "transient failure, requeueing event", "error", err)
		time.Sleep(time.Second)
		c.reject(ctx, delivery, true)
	}
}

var errUnknownEvent = errors.New("unknown event type")

func (c *Consumer) apply(ctx context.Context, env envelope) error {
	switch env.Type {
	case RoutingKeyCreateRequested:
		var payload moviePayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return fmt.Errorf("%w: bad create payload: %s", errUnknownEvent, err)
		}
		return c.applier.ApplyCreate(ctx, payload.toDomain())
	case RoutingKeyDeleteRequested:
		var payload deletePayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return fmt.Errorf("%w: bad delete payload: %s", errUnknownEvent, err)
		}
		return c.applier.ApplyDelete(ctx, payload.ID)
	default:
		return fmt.Errorf("%w: %s", errUnknownEvent, env.Type)
	}
}

func (c *Consumer) reject(ctx context.Context, delivery amqp.Delivery, requeue bool) {
	if err := delivery.Nack(false, requeue); err != nil {
		c.logger.ErrorContext(ctx, "failed to nack event", "error", err)
	}
}
