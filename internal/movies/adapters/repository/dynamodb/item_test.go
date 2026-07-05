package dynamodb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

func TestItemRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 123456789, time.UTC)
	movie := domain.Movie{
		ID:              "movie-1",
		Title:           "The Matrix",
		Year:            1999,
		Cast:            []string{"Keanu Reeves"},
		Genres:          []string{"Action"},
		Href:            "The_Matrix",
		Extract:         "A hacker discovers reality is a simulation.",
		Thumbnail:       "https://example.com/matrix.jpg",
		ThumbnailWidth:  220,
		ThumbnailHeight: 325,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	got, err := fromDomain(movie).toDomain()

	require.NoError(t, err)
	assert.Equal(t, movie, got)
}

func TestItemDerivedAttributes(t *testing.T) {
	movie := domain.Movie{
		ID:     "movie-1",
		Title:  "The Matrix",
		Genres: []string{"Action", "action", "Sci-Fi"},
	}

	item := fromDomain(movie)

	assert.Equal(t, gsiPartitionValue, item.GsiPK)
	assert.Equal(t, "the matrix"+titleSortSeparator+"movie-1", item.TitleSort)
	assert.Equal(t, "the matrix", item.TitleLC)
	// deduplicated after lowercasing: DynamoDB string sets reject duplicates
	assert.Equal(t, []string{"action", "sci-fi"}, item.GenresLC)
}

func TestItemDerivedAttributes_NoGenres(t *testing.T) {
	item := fromDomain(domain.Movie{ID: "movie-1", Title: "Alien"})

	assert.Empty(t, item.GenresLC)
}

func TestItemToDomain_EmptyTimestamps(t *testing.T) {
	item := movieItem{ID: "movie-1", Title: "The Matrix", Year: 1999}

	got, err := item.toDomain()

	require.NoError(t, err)
	assert.True(t, got.CreatedAt.IsZero())
	assert.True(t, got.UpdatedAt.IsZero())
}

func TestItemToDomain_InvalidTimestampFails(t *testing.T) {
	item := movieItem{ID: "movie-1", CreatedAt: "not-a-date"}

	_, err := item.toDomain()

	assert.Error(t, err)
}
