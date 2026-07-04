package memory

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

type Repository struct {
	mu     sync.RWMutex
	movies map[string]domain.Movie
}

var _ ports.MovieRepository = (*Repository)(nil)

func New() *Repository {
	return &Repository{movies: make(map[string]domain.Movie)}
}

func (r *Repository) List(_ context.Context, filter domain.ListFilter) (domain.MoviePage, error) {
	filter = filter.Normalized()

	r.mu.RLock()
	matched := make([]domain.Movie, 0, len(r.movies))
	for _, m := range r.movies {
		if filter.Matches(m) {
			matched = append(matched, clone(m))
		}
	}
	r.mu.RUnlock()

	slices.SortFunc(matched, func(a, b domain.Movie) int {
		if c := strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})

	start := min(filter.Offset(), len(matched))
	end := min(start+filter.PageSize, len(matched))
	return domain.MoviePage{
		Movies:   matched[start:end],
		Total:    int64(len(matched)),
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

func (r *Repository) GetByID(_ context.Context, id string) (domain.Movie, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.movies[id]
	if !ok {
		return domain.Movie{}, domain.ErrNotFound
	}
	return clone(m), nil
}

func (r *Repository) Create(_ context.Context, movie domain.Movie) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.movies[movie.ID]; ok {
		return domain.ErrAlreadyExists
	}
	r.movies[movie.ID] = clone(movie)
	return nil
}

func (r *Repository) CreateMany(_ context.Context, movies []domain.Movie) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range movies {
		r.movies[m.ID] = clone(m)
	}
	return nil
}

func (r *Repository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.movies[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.movies, id)
	return nil
}

func clone(m domain.Movie) domain.Movie {
	m.Cast = slices.Clone(m.Cast)
	m.Genres = slices.Clone(m.Genres)
	return m
}
