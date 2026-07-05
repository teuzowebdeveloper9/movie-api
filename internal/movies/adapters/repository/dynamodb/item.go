package dynamodb

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

// titleSortSeparator joins lowercase title and id in title_sort. 0x1F (unit
// separator) sorts below every printable character, so composite keys keep the
// exact (title, id) ordering even when one title is a prefix of another.
const titleSortSeparator = "\x1f"

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

	// Derived attributes backing the title-sort GSI and its server-side
	// filters; write-only (toDomain ignores them, List never reads them back).
	GsiPK     string   `dynamodbav:"gsi_pk"`
	TitleSort string   `dynamodbav:"title_sort"`
	TitleLC   string   `dynamodbav:"title_lc"`
	GenresLC  []string `dynamodbav:"genres_lc,stringset,omitempty"`
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
		GsiPK:           gsiPartitionValue,
		TitleSort:       strings.ToLower(m.Title) + titleSortSeparator + m.ID,
		TitleLC:         strings.ToLower(m.Title),
		GenresLC:        lowerSet(m.Genres),
	}
}

// lowerSet lowercases values and removes duplicates: genres_lc is a DynamoDB
// String Set, which rejects writes containing repeated members.
func lowerSet(values []string) []string {
	set := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.ToLower(v)
		if !slices.Contains(set, v) {
			set = append(set, v)
		}
	}
	return set
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
