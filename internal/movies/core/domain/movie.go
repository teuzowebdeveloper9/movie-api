package domain

import (
	"fmt"
	"strings"
	"time"
)

const (
	firstMovieYear = 1888

	maxYearsAhead = 10

	maxTitleLength = 500
)

type Movie struct {
	ID              string
	Title           string
	Year            int
	Cast            []string
	Genres          []string
	Href            string
	Extract         string
	Thumbnail       string
	ThumbnailWidth  int
	ThumbnailHeight int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type NewMovieInput struct {
	Title           string
	Year            int
	Cast            []string
	Genres          []string
	Href            string
	Extract         string
	Thumbnail       string
	ThumbnailWidth  int
	ThumbnailHeight int
}

func NewMovie(id string, in NewMovieInput, now time.Time) (Movie, error) {
	m := Movie{
		ID:              strings.TrimSpace(id),
		Title:           strings.TrimSpace(in.Title),
		Year:            in.Year,
		Cast:            cleanList(in.Cast),
		Genres:          cleanList(in.Genres),
		Href:            strings.TrimSpace(in.Href),
		Extract:         strings.TrimSpace(in.Extract),
		Thumbnail:       strings.TrimSpace(in.Thumbnail),
		ThumbnailWidth:  in.ThumbnailWidth,
		ThumbnailHeight: in.ThumbnailHeight,
		CreatedAt:       now.UTC(),
		UpdatedAt:       now.UTC(),
	}
	if err := m.Validate(now); err != nil {
		return Movie{}, err
	}
	return m, nil
}

func (m Movie) Validate(now time.Time) error {
	var violations []Violation
	if m.ID == "" {
		violations = append(violations, Violation{Field: "id", Message: "must not be empty"})
	}
	if m.Title == "" {
		violations = append(violations, Violation{Field: "title", Message: "must not be empty"})
	}
	if len(m.Title) > maxTitleLength {
		violations = append(violations, Violation{
			Field:   "title",
			Message: fmt.Sprintf("must have at most %d characters", maxTitleLength),
		})
	}
	if maxYear := now.Year() + maxYearsAhead; m.Year < firstMovieYear || m.Year > maxYear {
		violations = append(violations, Violation{
			Field:   "year",
			Message: fmt.Sprintf("must be between %d and %d", firstMovieYear, maxYear),
		})
	}
	if m.ThumbnailWidth < 0 {
		violations = append(violations, Violation{Field: "thumbnail_width", Message: "must not be negative"})
	}
	if m.ThumbnailHeight < 0 {
		violations = append(violations, Violation{Field: "thumbnail_height", Message: "must not be negative"})
	}
	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
	}
	return nil
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
