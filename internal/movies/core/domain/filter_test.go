package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

func TestListFilter_Normalized(t *testing.T) {
	tests := []struct {
		name     string
		in       domain.ListFilter
		wantPage int
		wantSize int
	}{
		{name: "zero values get defaults", in: domain.ListFilter{}, wantPage: 1, wantSize: domain.DefaultPageSize},
		{name: "negative values get defaults", in: domain.ListFilter{Page: -3, PageSize: -1}, wantPage: 1, wantSize: domain.DefaultPageSize},
		{name: "oversized page size is capped", in: domain.ListFilter{Page: 2, PageSize: 9999}, wantPage: 2, wantSize: domain.MaxPageSize},
		{name: "valid values pass through", in: domain.ListFilter{Page: 3, PageSize: 10}, wantPage: 3, wantSize: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Normalized()
			assert.Equal(t, tt.wantPage, got.Page)
			assert.Equal(t, tt.wantSize, got.PageSize)
		})
	}
}

func TestListFilter_NormalizedTrimsSearchTerms(t *testing.T) {
	got := domain.ListFilter{Title: "  matrix ", Genre: " Drama  "}.Normalized()

	assert.Equal(t, "matrix", got.Title)
	assert.Equal(t, "Drama", got.Genre)
}

func TestListFilter_Offset(t *testing.T) {
	filter := domain.ListFilter{Page: 3, PageSize: 10}.Normalized()
	assert.Equal(t, 20, filter.Offset())
}

func TestListFilter_Matches(t *testing.T) {
	movie := domain.Movie{
		Title:  "The Matrix",
		Year:   1999,
		Genres: []string{"Action", "Science Fiction"},
	}

	tests := []struct {
		name   string
		filter domain.ListFilter
		want   bool
	}{
		{name: "empty filter matches", filter: domain.ListFilter{}, want: true},
		{name: "title substring case-insensitive", filter: domain.ListFilter{Title: "matrix"}, want: true},
		{name: "title mismatch", filter: domain.ListFilter{Title: "inception"}, want: false},
		{name: "genre exact case-insensitive", filter: domain.ListFilter{Genre: "action"}, want: true},
		{name: "genre partial does not match", filter: domain.ListFilter{Genre: "Act"}, want: false},
		{name: "year match", filter: domain.ListFilter{Year: 1999}, want: true},
		{name: "year mismatch", filter: domain.ListFilter{Year: 2000}, want: false},
		{name: "all criteria combined", filter: domain.ListFilter{Title: "the", Genre: "Science Fiction", Year: 1999}, want: true},
		{name: "one failing criterion fails all", filter: domain.ListFilter{Title: "the", Genre: "Drama", Year: 1999}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.filter.Matches(movie))
		})
	}
}
