package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

// chunkSize bounds how many movies are handed to CreateMany at once so a
// large seed file doesn't turn into a single oversized bulk write.
const chunkSize = 1000

type movieJSON struct {
	ID              json.Number `json:"id"`
	Title           string      `json:"title"`
	Year            flexYear    `json:"year"`
	Cast            []string    `json:"cast"`
	Genres          []string    `json:"genres"`
	Href            string      `json:"href"`
	Extract         string      `json:"extract"`
	Thumbnail       string      `json:"thumbnail"`
	ThumbnailWidth  int         `json:"thumbnail_width"`
	ThumbnailHeight int         `json:"thumbnail_height"`
}

// flexYear accepts the year both as a JSON number (legacy format) and as a
// string ("1894", the format of the provided movies.json). Unparseable values
// decode to zero so the entry is rejected by domain validation instead of
// aborting the whole seed file.
type flexYear int

func (y *flexYear) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(strings.Trim(string(data), `"`))
	if s == "" || s == "null" {
		*y = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		*y = 0
		return nil
	}
	*y = flexYear(n)
	return nil
}

func Run(ctx context.Context, repo ports.MovieRepository, path string, logger *slog.Logger) error {
	page, err := repo.List(ctx, domain.ListFilter{Page: 1, PageSize: 1})
	if err != nil {
		return fmt.Errorf("checking repository before seed: %w", err)
	}
	if page.Total > 0 {
		logger.InfoContext(ctx, "seed skipped, repository already populated", "total", page.Total)
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading seed file: %w", err)
	}
	var entries []movieJSON
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parsing seed file: %w", err)
	}

	now := time.Now().UTC()
	movies := make([]domain.Movie, 0, len(entries))
	skipped := 0
	for _, e := range entries {
		id := e.ID.String()
		if id == "" {
			id = uuid.NewString()
		}
		movie, err := domain.NewMovie(id, domain.NewMovieInput{
			Title:           e.Title,
			Year:            int(e.Year),
			Cast:            e.Cast,
			Genres:          e.Genres,
			Href:            e.Href,
			Extract:         e.Extract,
			Thumbnail:       e.Thumbnail,
			ThumbnailWidth:  e.ThumbnailWidth,
			ThumbnailHeight: e.ThumbnailHeight,
		}, now)
		if err != nil {
			logger.WarnContext(ctx, "skipping invalid seed entry", "title", e.Title, "error", err)
			skipped++
			continue
		}
		movies = append(movies, movie)
	}

	for start := 0; start < len(movies); start += chunkSize {
		end := min(start+chunkSize, len(movies))
		if err := repo.CreateMany(ctx, movies[start:end]); err != nil {
			return fmt.Errorf("seeding movies %d-%d: %w", start+1, end, err)
		}
	}
	logger.InfoContext(ctx, "seed finished",
		"seeded", len(movies), "skipped", skipped, "total_entries", len(entries))
	return nil
}
