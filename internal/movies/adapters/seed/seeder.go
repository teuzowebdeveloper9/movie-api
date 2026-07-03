package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

type movieJSON struct {
	Title           string   `json:"title"`
	Year            int      `json:"year"`
	Cast            []string `json:"cast"`
	Genres          []string `json:"genres"`
	Href            string   `json:"href"`
	Extract         string   `json:"extract"`
	Thumbnail       string   `json:"thumbnail"`
	ThumbnailWidth  int      `json:"thumbnail_width"`
	ThumbnailHeight int      `json:"thumbnail_height"`
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
	seeded := 0
	for _, e := range entries {
		movie, err := domain.NewMovie(uuid.NewString(), domain.NewMovieInput{
			Title:           e.Title,
			Year:            e.Year,
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
			continue
		}
		if err := repo.Create(ctx, movie); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
			return fmt.Errorf("seeding movie %q: %w", movie.Title, err)
		}
		seeded++
	}
	logger.InfoContext(ctx, "seed finished", "seeded", seeded, "total_entries", len(entries))
	return nil
}
