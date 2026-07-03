package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

type Publisher struct {
	conn   *amqp.Connection
	ch     *amqp.Channel
	mu     sync.Mutex
	logger *slog.Logger
}

var _ ports.EventPublisher = (*Publisher)(nil)

func NewPublisher(ctx context.Context, url string, logger *slog.Logger) (*Publisher, error) {
	conn, err := dialWithRetry(ctx, url, logger)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("opening channel: %w", err)
	}
	if err := declareTopology(ch); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("enabling publisher confirms: %w", err)
	}
	return &Publisher{conn: conn, ch: ch, logger: logger}, nil
}

func (p *Publisher) MovieCreateRequested(ctx context.Context, movie domain.Movie) error {
	return p.publish(ctx, RoutingKeyCreateRequested, payloadFromDomain(movie))
}

func (p *Publisher) MovieDeleteRequested(ctx context.Context, id string) error {
	return p.publish(ctx, RoutingKeyDeleteRequested, deletePayload{ID: id})
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ch.Close(); err != nil {
		_ = p.conn.Close()
		return fmt.Errorf("closing channel: %w", err)
	}
	return p.conn.Close()
}

func (p *Publisher) publish(ctx context.Context, key string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding payload: %w", err)
	}
	eventID := uuid.NewString()
	env, err := json.Marshal(envelope{
		EventID:    eventID,
		Type:       key,
		OccurredAt: time.Now().UTC(),
		Payload:    body,
	})
	if err != nil {
		return fmt.Errorf("encoding envelope: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	confirmation, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, Exchange, key, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    eventID,
		Timestamp:    time.Now().UTC(),
		Body:         env,
	})
	if err != nil {
		return fmt.Errorf("publishing %s: %w", key, err)
	}
	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("waiting broker confirmation for %s: %w", key, err)
	}
	if !acked {
		return fmt.Errorf("broker rejected event %s (%s)", eventID, key)
	}
	p.logger.DebugContext(ctx, "event published", "event_id", eventID, "type", key)
	return nil
}
