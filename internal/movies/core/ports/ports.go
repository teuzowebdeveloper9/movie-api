package ports

import (
	"context"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

type MovieRepository interface {
	List(ctx context.Context, filter domain.ListFilter) (domain.MoviePage, error)
	GetByID(ctx context.Context, id string) (domain.Movie, error)
	Create(ctx context.Context, movie domain.Movie) error
	Delete(ctx context.Context, id string) error
}

type EventPublisher interface {
	MovieCreateRequested(ctx context.Context, movie domain.Movie) error
	MovieDeleteRequested(ctx context.Context, id string) error
}

type WriteOutcome int

const (
	WriteCompleted WriteOutcome = iota + 1

	WriteAccepted
)

type MovieService interface {
	List(ctx context.Context, filter domain.ListFilter) (domain.MoviePage, error)
	Get(ctx context.Context, id string) (domain.Movie, error)
	Create(ctx context.Context, input domain.NewMovieInput) (domain.Movie, WriteOutcome, error)
	Delete(ctx context.Context, id string) (WriteOutcome, error)
}

type MovieEventApplier interface {
	ApplyCreate(ctx context.Context, movie domain.Movie) error
	ApplyDelete(ctx context.Context, id string) error
}
