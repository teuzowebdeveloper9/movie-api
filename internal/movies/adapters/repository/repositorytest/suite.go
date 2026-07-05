// Package repositorytest provides a black-box conformance suite that every
// ports.MovieRepository adapter must satisfy, keeping observable behavior
// (ordering, filtering, pagination, error semantics) identical across
// drivers. It is exercised by the adapter integration tests (build tag
// "integration") against real backends via testcontainers.
package repositorytest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

// seedTime has millisecond precision so it survives BSON datetime roundtrips.
var seedTime = time.Date(2026, 7, 1, 10, 30, 0, int(500*time.Millisecond), time.UTC)

// seedMovies returns the fixture dataset. Titles are ASCII, Title Case and
// start with distinct letters on purpose: binary ordering (MongoDB) and
// case-insensitive ordering (DynamoDB title_sort) agree, so every adapter can
// be held to the same expected order.
func seedMovies() []domain.Movie {
	newMovie := func(id, title string, year int, cast, genres []string) domain.Movie {
		return domain.Movie{
			ID:        id,
			Title:     title,
			Year:      year,
			Cast:      cast,
			Genres:    genres,
			CreatedAt: seedTime,
			UpdatedAt: seedTime,
		}
	}
	return []domain.Movie{
		newMovie("1", "Alien", 1979, []string{"Sigourney Weaver"}, []string{"Horror", "Sci-Fi"}),
		newMovie("2", "Blade Runner", 1982, []string{"Harrison Ford"}, []string{"Sci-Fi"}),
		newMovie("3", "Casablanca", 1942, []string{"Humphrey Bogart"}, []string{"Drama", "Romance"}),
		newMovie("4", "Drive", 2011, []string{"Ryan Gosling"}, []string{"Action", "Drama"}),
		newMovie("5", "Eraserhead", 1977, []string{"Jack Nance"}, []string{"Horror"}),
	}
}

// Run seeds the repository and exercises the full MovieRepository contract.
// The repository must be empty; mutating subtests restore the seeded state.
func Run(t *testing.T, repo ports.MovieRepository) {
	ctx := context.Background()
	seed := seedMovies()
	require.NoError(t, repo.CreateMany(ctx, seed))

	t.Run("GetByID returns the stored movie", func(t *testing.T) {
		got, err := repo.GetByID(ctx, "1")
		require.NoError(t, err)
		assertMovieEqual(t, seed[0], got)
	})

	t.Run("GetByID with unknown id returns ErrNotFound", func(t *testing.T) {
		_, err := repo.GetByID(ctx, "does-not-exist")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("List returns movies sorted by title with exact total", func(t *testing.T) {
		page, err := repo.List(ctx, domain.ListFilter{Page: 1, PageSize: 100})
		require.NoError(t, err)
		assert.Equal(t, int64(5), page.Total)
		assert.Equal(t,
			[]string{"Alien", "Blade Runner", "Casablanca", "Drive", "Eraserhead"},
			titlesOf(page.Movies))
	})

	t.Run("List paginates keeping the exact total", func(t *testing.T) {
		page, err := repo.List(ctx, domain.ListFilter{Page: 2, PageSize: 2})
		require.NoError(t, err)
		assert.Equal(t, int64(5), page.Total)
		assert.Equal(t, []string{"Casablanca", "Drive"}, titlesOf(page.Movies))

		empty, err := repo.List(ctx, domain.ListFilter{Page: 4, PageSize: 2})
		require.NoError(t, err)
		assert.Equal(t, int64(5), empty.Total)
		assert.Empty(t, empty.Movies)
	})

	t.Run("List filters by title substring case-insensitively", func(t *testing.T) {
		page, err := repo.List(ctx, domain.ListFilter{Title: "RUNNER"})
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
		assert.Equal(t, []string{"Blade Runner"}, titlesOf(page.Movies))
	})

	t.Run("List filters by genre case-insensitively", func(t *testing.T) {
		page, err := repo.List(ctx, domain.ListFilter{Genre: "horror"})
		require.NoError(t, err)
		assert.Equal(t, int64(2), page.Total)
		assert.Equal(t, []string{"Alien", "Eraserhead"}, titlesOf(page.Movies))
	})

	t.Run("List filters by year", func(t *testing.T) {
		page, err := repo.List(ctx, domain.ListFilter{Year: 1982})
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
		assert.Equal(t, []string{"Blade Runner"}, titlesOf(page.Movies))
	})

	t.Run("List combines filters", func(t *testing.T) {
		page, err := repo.List(ctx, domain.ListFilter{Genre: "drama", Year: 2011})
		require.NoError(t, err)
		assert.Equal(t, int64(1), page.Total)
		assert.Equal(t, []string{"Drive"}, titlesOf(page.Movies))
	})

	t.Run("Create persists, rejects duplicates and updates the total", func(t *testing.T) {
		movie := domain.Movie{
			ID:        "6",
			Title:     "Fargo",
			Year:      1996,
			Cast:      []string{"Frances McDormand"},
			Genres:    []string{"Crime"},
			CreatedAt: seedTime,
			UpdatedAt: seedTime,
		}
		require.NoError(t, repo.Create(ctx, movie))

		got, err := repo.GetByID(ctx, movie.ID)
		require.NoError(t, err)
		assertMovieEqual(t, movie, got)

		assert.ErrorIs(t, repo.Create(ctx, movie), domain.ErrAlreadyExists)

		page, err := repo.List(ctx, domain.ListFilter{Page: 1, PageSize: 100})
		require.NoError(t, err)
		assert.Equal(t, int64(6), page.Total)

		require.NoError(t, repo.Delete(ctx, movie.ID))
	})

	t.Run("Create without cast or genres roundtrips as empty", func(t *testing.T) {
		movie := domain.Movie{ID: "7", Title: "Gattaca", Year: 1997, CreatedAt: seedTime, UpdatedAt: seedTime}
		require.NoError(t, repo.Create(ctx, movie))

		got, err := repo.GetByID(ctx, movie.ID)
		require.NoError(t, err)
		assert.Empty(t, got.Cast)
		assert.Empty(t, got.Genres)

		page, err := repo.List(ctx, domain.ListFilter{Genre: "crime"})
		require.NoError(t, err)
		assert.Empty(t, titlesOf(page.Movies), "movie without genres must not match a genre filter")

		require.NoError(t, repo.Delete(ctx, movie.ID))
	})

	t.Run("CreateMany tolerates already-existing ids", func(t *testing.T) {
		fresh := domain.Movie{ID: "8", Title: "Heat", Year: 1995, CreatedAt: seedTime, UpdatedAt: seedTime}
		require.NoError(t, repo.CreateMany(ctx, []domain.Movie{seed[0], fresh}))

		got, err := repo.GetByID(ctx, fresh.ID)
		require.NoError(t, err)
		assert.Equal(t, fresh.Title, got.Title)

		require.NoError(t, repo.Delete(ctx, fresh.ID))
	})

	t.Run("Delete removes the movie", func(t *testing.T) {
		movie := domain.Movie{ID: "9", Title: "Ikiru", Year: 1952, CreatedAt: seedTime, UpdatedAt: seedTime}
		require.NoError(t, repo.Create(ctx, movie))
		require.NoError(t, repo.Delete(ctx, movie.ID))

		_, err := repo.GetByID(ctx, movie.ID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("Delete with unknown id returns ErrNotFound", func(t *testing.T) {
		assert.ErrorIs(t, repo.Delete(ctx, "does-not-exist"), domain.ErrNotFound)
	})
}

func titlesOf(movies []domain.Movie) []string {
	titles := make([]string, 0, len(movies))
	for _, m := range movies {
		titles = append(titles, m.Title)
	}
	return titles
}

// assertMovieEqual compares timestamps with tolerance (drivers differ in
// stored precision: BSON datetime is milliseconds, DynamoDB RFC3339Nano) and
// every other field exactly.
func assertMovieEqual(t *testing.T, want, got domain.Movie) {
	t.Helper()
	assert.WithinDuration(t, want.CreatedAt, got.CreatedAt, time.Second)
	assert.WithinDuration(t, want.UpdatedAt, got.UpdatedAt, time.Second)
	got.CreatedAt, got.UpdatedAt = want.CreatedAt, want.UpdatedAt
	assert.Equal(t, want, got)
}
