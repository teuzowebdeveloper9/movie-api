package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

var now = time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

func validInput() domain.NewMovieInput {
	return domain.NewMovieInput{
		Title:           "The Matrix",
		Year:            1999,
		Cast:            []string{"Keanu Reeves", "Carrie-Anne Moss"},
		Genres:          []string{"Action", "Science Fiction"},
		Href:            "The_Matrix",
		Extract:         "A hacker discovers reality is a simulation.",
		Thumbnail:       "https://example.com/matrix.jpg",
		ThumbnailWidth:  220,
		ThumbnailHeight: 325,
	}
}

func TestNewMovie_Valid(t *testing.T) {
	movie, err := domain.NewMovie("movie-1", validInput(), now)

	require.NoError(t, err)
	assert.Equal(t, "movie-1", movie.ID)
	assert.Equal(t, "The Matrix", movie.Title)
	assert.Equal(t, 1999, movie.Year)
	assert.Equal(t, now, movie.CreatedAt)
	assert.Equal(t, now, movie.UpdatedAt)
}

func TestNewMovie_NormalizesInput(t *testing.T) {
	in := validInput()
	in.Title = "  The Matrix  "
	in.Cast = []string{" Keanu Reeves ", "", "   "}
	in.Genres = []string{"", "Action "}

	movie, err := domain.NewMovie("  movie-1 ", in, now)

	require.NoError(t, err)
	assert.Equal(t, "movie-1", movie.ID)
	assert.Equal(t, "The Matrix", movie.Title)
	assert.Equal(t, []string{"Keanu Reeves"}, movie.Cast)
	assert.Equal(t, []string{"Action"}, movie.Genres)
}

func TestNewMovie_Violations(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		mutate func(*domain.NewMovieInput)
		field  string
	}{
		{
			name:   "empty id",
			id:     "   ",
			mutate: func(*domain.NewMovieInput) {},
			field:  "id",
		},
		{
			name:   "empty title",
			id:     "movie-1",
			mutate: func(in *domain.NewMovieInput) { in.Title = "  " },
			field:  "title",
		},
		{
			name:   "year before first movie ever",
			id:     "movie-1",
			mutate: func(in *domain.NewMovieInput) { in.Year = 1800 },
			field:  "year",
		},
		{
			name:   "year too far in the future",
			id:     "movie-1",
			mutate: func(in *domain.NewMovieInput) { in.Year = now.Year() + 50 },
			field:  "year",
		},
		{
			name:   "negative thumbnail width",
			id:     "movie-1",
			mutate: func(in *domain.NewMovieInput) { in.ThumbnailWidth = -1 },
			field:  "thumbnail_width",
		},
		{
			name:   "negative thumbnail height",
			id:     "movie-1",
			mutate: func(in *domain.NewMovieInput) { in.ThumbnailHeight = -10 },
			field:  "thumbnail_height",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)

			_, err := domain.NewMovie(tt.id, in, now)

			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalid)

			var validationErr *domain.ValidationError
			require.ErrorAs(t, err, &validationErr)
			fields := make([]string, 0, len(validationErr.Violations))
			for _, v := range validationErr.Violations {
				fields = append(fields, v.Field)
			}
			assert.Contains(t, fields, tt.field)
		})
	}
}

func TestNewMovie_AggregatesAllViolations(t *testing.T) {
	in := validInput()
	in.Title = ""
	in.Year = 0
	in.ThumbnailWidth = -1

	_, err := domain.NewMovie("", in, now)

	var validationErr *domain.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Len(t, validationErr.Violations, 4)
	assert.Contains(t, err.Error(), "title")
	assert.Contains(t, err.Error(), "year")
}

func TestValidationError_IsOnlyMatchesErrInvalid(t *testing.T) {
	err := &domain.ValidationError{Violations: []domain.Violation{{Field: "title", Message: "must not be empty"}}}

	assert.ErrorIs(t, err, domain.ErrInvalid)
	assert.False(t, errors.Is(err, domain.ErrNotFound))
}
