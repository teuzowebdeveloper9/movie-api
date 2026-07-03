package dynamodb

import (
	"fmt"
	"strings"
	"time"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

type movieItem struct {
	ID              string   `dynamodbav:"id"`
	Title           string   `dynamodbav:"title"`
	Year            int      `dynamodbav:"year"`
	Cast            []string `dynamodbav:"cast,omitempty"`
	Genres          []string `dynamodbav:"genres,omitempty"`
	Href            string   `dynamodbav:"href,omitempty"`
	Extract         string   `dynamodbav:"extract,omitempty"`
	Thumbnail       string   `dynamodbav:"thumbnail,omitempty"`
	ThumbnailWidth  int      `dynamodbav:"thumbnail_width,omitempty"`
	ThumbnailHeight int      `dynamodbav:"thumbnail_height,omitempty"`
	CreatedAt       string   `dynamodbav:"created_at"`
	UpdatedAt       string   `dynamodbav:"updated_at"`
}

func fromDomain(m domain.Movie) movieItem {
	return movieItem{
		ID:              m.ID,
		Title:           m.Title,
		Year:            m.Year,
		Cast:            m.Cast,
		Genres:          m.Genres,
		Href:            m.Href,
		Extract:         m.Extract,
		Thumbnail:       m.Thumbnail,
		ThumbnailWidth:  m.ThumbnailWidth,
		ThumbnailHeight: m.ThumbnailHeight,
		CreatedAt:       m.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:       m.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (i movieItem) toDomain() (domain.Movie, error) {
	createdAt, err := parseTime(i.CreatedAt)
	if err != nil {
		return domain.Movie{}, fmt.Errorf("parsing created_at of movie %q: %w", i.ID, err)
	}
	updatedAt, err := parseTime(i.UpdatedAt)
	if err != nil {
		return domain.Movie{}, fmt.Errorf("parsing updated_at of movie %q: %w", i.ID, err)
	}
	return domain.Movie{
		ID:              i.ID,
		Title:           i.Title,
		Year:            i.Year,
		Cast:            i.Cast,
		Genres:          i.Genres,
		Href:            i.Href,
		Extract:         i.Extract,
		Thumbnail:       i.Thumbnail,
		ThumbnailWidth:  i.ThumbnailWidth,
		ThumbnailHeight: i.ThumbnailHeight,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

func parseTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}
