package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"context"

	"github.com/google/uuid"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

type Clock func() time.Time

type IDGenerator func() string

type MovieService struct {
	repo      ports.MovieRepository
	publisher ports.EventPublisher
	now       Clock
	newID     IDGenerator
	logger    *slog.Logger
}

var (
	_ ports.MovieService      = (*MovieService)(nil)
	_ ports.MovieEventApplier = (*MovieService)(nil)
)

type Option func(*MovieService)

func WithPublisher(p ports.EventPublisher) Option {
	return func(s *MovieService) { s.publisher = p }
}

func WithClock(c Clock) Option {
	return func(s *MovieService) { s.now = c }
}

func WithIDGenerator(g IDGenerator) Option {
	return func(s *MovieService) { s.newID = g }
}

func WithLogger(l *slog.Logger) Option {
	return func(s *MovieService) { s.logger = l }
}

func New(repo ports.MovieRepository, opts ...Option) *MovieService {
	s := &MovieService{
		repo:   repo,
		now:    time.Now,
		newID:  uuid.NewString,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *MovieService) List(ctx context.Context, filter domain.ListFilter) (domain.MoviePage, error) {
	page, err := s.repo.List(ctx, filter.Normalized())
	if err != nil {
		return domain.MoviePage{}, fmt.Errorf("listing movies: %w", err)
	}
	return page, nil
}

func (s *MovieService) Get(ctx context.Context, id string) (domain.Movie, error) {
	id, err := requireID(id)
	if err != nil {
		return domain.Movie{}, err
	}
	movie, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Movie{}, fmt.Errorf("getting movie %q: %w", id, err)
	}
	return movie, nil
}

func (s *MovieService) Create(ctx context.Context, input domain.NewMovieInput) (domain.Movie, ports.WriteOutcome, error) {
	movie, err := domain.NewMovie(s.newID(), input, s.now())
	if err != nil {
		return domain.Movie{}, 0, err
	}

	if s.publisher != nil {
		if err := s.publisher.MovieCreateRequested(ctx, movie); err != nil {
			return domain.Movie{}, 0, fmt.Errorf("publishing movie create request: %w", err)
		}
		s.logger.InfoContext(ctx, "movie create accepted for async processing", "movie_id", movie.ID)
		return movie, ports.WriteAccepted, nil
	}

	if err := s.repo.Create(ctx, movie); err != nil {
		return domain.Movie{}, 0, fmt.Errorf("creating movie: %w", err)
	}
	s.logger.InfoContext(ctx, "movie created", "movie_id", movie.ID)
	return movie, ports.WriteCompleted, nil
}

func (s *MovieService) Delete(ctx context.Context, id string) (ports.WriteOutcome, error) {
	id, err := requireID(id)
	if err != nil {
		return 0, err
	}
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return 0, fmt.Errorf("checking movie %q: %w", id, err)
	}

	if s.publisher != nil {
		if err := s.publisher.MovieDeleteRequested(ctx, id); err != nil {
			return 0, fmt.Errorf("publishing movie delete request: %w", err)
		}
		s.logger.InfoContext(ctx, "movie delete accepted for async processing", "movie_id", id)
		return ports.WriteAccepted, nil
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return 0, fmt.Errorf("deleting movie: %w", err)
	}
	s.logger.InfoContext(ctx, "movie deleted", "movie_id", id)
	return ports.WriteCompleted, nil
}

func (s *MovieService) ApplyCreate(ctx context.Context, movie domain.Movie) error {
	if err := movie.Validate(s.now()); err != nil {
		return err
	}
	err := s.repo.Create(ctx, movie)
	if errors.Is(err, domain.ErrAlreadyExists) {
		s.logger.WarnContext(ctx, "movie already created, skipping redelivered event", "movie_id", movie.ID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("applying movie create: %w", err)
	}
	s.logger.InfoContext(ctx, "movie created from event", "movie_id", movie.ID)
	return nil
}

func (s *MovieService) ApplyDelete(ctx context.Context, id string) error {
	err := s.repo.Delete(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.logger.WarnContext(ctx, "movie already deleted, skipping redelivered event", "movie_id", id)
		return nil
	}
	if err != nil {
		return fmt.Errorf("applying movie delete: %w", err)
	}
	s.logger.InfoContext(ctx, "movie deleted from event", "movie_id", id)
	return nil
}

func requireID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", &domain.ValidationError{Violations: []domain.Violation{{Field: "id", Message: "must not be empty"}}}
	}
	return id, nil
}
