package memory_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/memory"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

func newMovie(t *testing.T, id, title string, year int, genres ...string) domain.Movie {
	t.Helper()
	movie, err := domain.NewMovie(id, domain.NewMovieInput{
		Title:  title,
		Year:   year,
		Cast:   []string{"Someone"},
		Genres: genres,
	}, time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	return movie
}

func TestRepository_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	movie := newMovie(t, "movie-1", "The Matrix", 1999, "Action")

	require.NoError(t, repo.Create(ctx, movie))

	got, err := repo.GetByID(ctx, "movie-1")
	require.NoError(t, err)
	assert.Equal(t, movie, got)
}

func TestRepository_GetMissingReturnsNotFound(t *testing.T) {
	repo := memory.New()

	_, err := repo.GetByID(context.Background(), "ghost")

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRepository_CreateDuplicateReturnsAlreadyExists(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	movie := newMovie(t, "movie-1", "The Matrix", 1999)

	require.NoError(t, repo.Create(ctx, movie))

	assert.ErrorIs(t, repo.Create(ctx, movie), domain.ErrAlreadyExists)
}

func TestRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	require.NoError(t, repo.Create(ctx, newMovie(t, "movie-1", "The Matrix", 1999)))

	require.NoError(t, repo.Delete(ctx, "movie-1"))

	_, err := repo.GetByID(ctx, "movie-1")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRepository_DeleteMissingReturnsNotFound(t *testing.T) {
	repo := memory.New()

	assert.ErrorIs(t, repo.Delete(context.Background(), "ghost"), domain.ErrNotFound)
}

func TestRepository_ListFiltersAndSorts(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	require.NoError(t, repo.Create(ctx, newMovie(t, "movie-1", "Se7en", 1995, "Crime")))
	require.NoError(t, repo.Create(ctx, newMovie(t, "movie-2", "The Matrix", 1999, "Action")))
	require.NoError(t, repo.Create(ctx, newMovie(t, "movie-3", "Inception", 2010, "Action")))

	page, err := repo.List(ctx, domain.ListFilter{Genre: "action"})
	require.NoError(t, err)

	assert.EqualValues(t, 2, page.Total)
	require.Len(t, page.Movies, 2)
	assert.Equal(t, "Inception", page.Movies[0].Title)
	assert.Equal(t, "The Matrix", page.Movies[1].Title)
}

func TestRepository_ListByTitleAndYear(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	require.NoError(t, repo.Create(ctx, newMovie(t, "movie-1", "The Matrix", 1999, "Action")))
	require.NoError(t, repo.Create(ctx, newMovie(t, "movie-2", "The Matrix Reloaded", 2003, "Action")))

	page, err := repo.List(ctx, domain.ListFilter{Title: "matrix", Year: 2003})
	require.NoError(t, err)

	assert.EqualValues(t, 1, page.Total)
	require.Len(t, page.Movies, 1)
	assert.Equal(t, "The Matrix Reloaded", page.Movies[0].Title)
}

func TestRepository_ListPaginates(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	for i := range 5 {
		id := fmt.Sprintf("movie-%d", i)
		title := fmt.Sprintf("Movie %d", i)
		require.NoError(t, repo.Create(ctx, newMovie(t, id, title, 2000+i)))
	}

	page, err := repo.List(ctx, domain.ListFilter{Page: 2, PageSize: 2})
	require.NoError(t, err)

	assert.EqualValues(t, 5, page.Total)
	require.Len(t, page.Movies, 2)
	assert.Equal(t, "Movie 2", page.Movies[0].Title)
	assert.Equal(t, "Movie 3", page.Movies[1].Title)

	empty, err := repo.List(ctx, domain.ListFilter{Page: 4, PageSize: 2})
	require.NoError(t, err)
	assert.Empty(t, empty.Movies)
	assert.EqualValues(t, 5, empty.Total)
}

func TestRepository_ReturnsIsolatedCopies(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	require.NoError(t, repo.Create(ctx, newMovie(t, "movie-1", "The Matrix", 1999, "Action")))

	got, err := repo.GetByID(ctx, "movie-1")
	require.NoError(t, err)
	got.Genres[0] = "Corrupted"

	fresh, err := repo.GetByID(ctx, "movie-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"Action"}, fresh.Genres)
}
