//go:build integration

package rabbitmq_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcrabbitmq "github.com/testcontainers/testcontainers-go/modules/rabbitmq"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/messaging/rabbitmq"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

// rabbitImage matches docker-compose.yml and deploy/k8s/rabbitmq.yaml so the
// suite runs against the same broker the application ships with.
const rabbitImage = "rabbitmq:3.13-management-alpine"

// dockerInit runs docker-init (tini) as the container's PID 1 so signals are
// forwarded and zombies reaped — without it, some hosts cannot stop the
// container at cleanup ("PID ... is zombie and can not be killed").
func dockerInit() testcontainers.CustomizeRequestOption {
	return testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
		enabled := true
		hc.Init = &enabled
	})
}

type appliedEvent struct {
	movie     domain.Movie
	deletedID string
}

type recordingApplier struct {
	events chan appliedEvent
}

func (r *recordingApplier) ApplyCreate(_ context.Context, m domain.Movie) error {
	r.events <- appliedEvent{movie: m}
	return nil
}

func (r *recordingApplier) ApplyDelete(_ context.Context, id string) error {
	r.events <- appliedEvent{deletedID: id}
	return nil
}

func TestPublisherConsumerIntegration(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcrabbitmq.Run(ctx, rabbitImage, dockerInit())
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	url, err := ctr.AmqpURL(ctx)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publisher, err := rabbitmq.NewPublisher(ctx, url, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = publisher.Close() })

	applier := &recordingApplier{events: make(chan appliedEvent, 16)}
	consumer := rabbitmq.NewConsumer(url, applier, logger)
	consumerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = consumer.Run(consumerCtx)
	}()
	// stop the consumer before the container goes away, or Run's reconnect
	// loop races the teardown
	t.Cleanup(func() { cancel(); <-done })

	movie := domain.Movie{
		ID:        "movie-1",
		Title:     "The Matrix",
		Year:      1999,
		Cast:      []string{"Keanu Reeves"},
		Genres:    []string{"Action"},
		CreatedAt: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
	}

	t.Run("create request reaches the applier", func(t *testing.T) {
		require.NoError(t, publisher.MovieCreateRequested(ctx, movie))

		got := waitEvent(t, applier.events)
		assert.Equal(t, movie, got.movie)
	})

	t.Run("delete request reaches the applier", func(t *testing.T) {
		require.NoError(t, publisher.MovieDeleteRequested(ctx, "movie-1"))

		got := waitEvent(t, applier.events)
		assert.Equal(t, "movie-1", got.deletedID)
	})

	t.Run("poison message lands in the DLQ without reaching the applier", func(t *testing.T) {
		conn, err := amqp.Dial(url)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		ch, err := conn.Channel()
		require.NoError(t, err)
		defer func() { _ = ch.Close() }()

		// valid envelope with an unknown event type: the consumer must reject
		// it without requeue, routing it to the DLQ via the dead-letter
		// exchange instead of blocking the queue
		poison, err := json.Marshal(map[string]any{
			"event_id": "poison-1",
			"type":     "movie.exploded",
			"payload":  map[string]any{},
		})
		require.NoError(t, err)
		require.NoError(t, ch.PublishWithContext(ctx,
			rabbitmq.Exchange, rabbitmq.RoutingKeyCreateRequested, false, false,
			amqp.Publishing{ContentType: "application/json", Body: poison}))

		deadline := time.Now().Add(15 * time.Second)
		for {
			msg, ok, err := ch.Get(rabbitmq.DLQueue, true)
			require.NoError(t, err)
			if ok {
				assert.Contains(t, string(msg.Body), "poison-1")
				break
			}
			require.False(t, time.Now().After(deadline), "poison message never reached the DLQ")
			time.Sleep(200 * time.Millisecond)
		}

		select {
		case got := <-applier.events:
			t.Fatalf("poison event must not reach the applier, got %+v", got)
		default:
		}
	})
}

func waitEvent(t *testing.T, events chan appliedEvent) appliedEvent {
	t.Helper()
	select {
	case e := <-events:
		return e
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for the applier to receive the event")
		return appliedEvent{}
	}
}
