package seed_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/memory"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/seed"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

func writeSeedFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "movies.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestRunSeedsOfficialFormat(t *testing.T) {
	// Same shape as the provided movies.json: numeric id, year as string.
	path := writeSeedFile(t, `[
		{"id": 8, "title": "Edison Kinetoscopic Record of a Sneeze (1894)", "year": "1894"},
		{"id": 10, "title": "La sortie des usines Lumière (1895)", "year": "1895"},
		{"id": 99, "title": "", "year": "2005"}
	]`)
	repo := memory.New()

	require.NoError(t, seed.Run(context.Background(), repo, path, discardLogger()))

	page, err := repo.List(context.Background(), domain.ListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 2, page.Total, "entry with empty title must be skipped")

	movie, err := repo.GetByID(context.Background(), "8")
	require.NoError(t, err, "original numeric id must be preserved as string")
	require.Equal(t, "Edison Kinetoscopic Record of a Sneeze (1894)", movie.Title)
	require.Equal(t, 1894, movie.Year, "string year must be parsed to int")
}

func TestRunSeedsLegacyRichFormat(t *testing.T) {
	path := writeSeedFile(t, `[
		{"title": "The Matrix", "year": 1999, "cast": ["Keanu Reeves"], "genres": ["Action"]}
	]`)
	repo := memory.New()

	require.NoError(t, seed.Run(context.Background(), repo, path, discardLogger()))

	page, err := repo.List(context.Background(), domain.ListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.NotEmpty(t, page.Movies[0].ID, "entry without id must receive a generated one")
	require.Equal(t, []string{"Keanu Reeves"}, page.Movies[0].Cast)
}

func TestRunSkipsWhenRepositoryPopulated(t *testing.T) {
	first := writeSeedFile(t, `[{"id": 1, "title": "First", "year": "2000"}]`)
	second := writeSeedFile(t, `[{"id": 2, "title": "Second", "year": "2001"}]`)
	repo := memory.New()

	require.NoError(t, seed.Run(context.Background(), repo, first, discardLogger()))
	require.NoError(t, seed.Run(context.Background(), repo, second, discardLogger()))

	page, err := repo.List(context.Background(), domain.ListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total, "second seed must be skipped")
	_, err = repo.GetByID(context.Background(), "2")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRunFailsOnMalformedFile(t *testing.T) {
	path := writeSeedFile(t, `{not json`)
	repo := memory.New()

	require.Error(t, seed.Run(context.Background(), repo, path, discardLogger()))
}
