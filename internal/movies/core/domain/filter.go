package domain

import (
	"slices"
	"strings"
)

const (
	DefaultPageSize = 20

	MaxPageSize = 100
)

type ListFilter struct {
	Page     int
	PageSize int
	Title    string
	Genre    string
	Year     int
}

func (f ListFilter) Normalized() ListFilter {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = DefaultPageSize
	}
	if f.PageSize > MaxPageSize {
		f.PageSize = MaxPageSize
	}
	f.Title = strings.TrimSpace(f.Title)
	f.Genre = strings.TrimSpace(f.Genre)
	return f
}

func (f ListFilter) Offset() int {
	return (f.Page - 1) * f.PageSize
}

func (f ListFilter) Matches(m Movie) bool {
	if f.Title != "" && !strings.Contains(strings.ToLower(m.Title), strings.ToLower(f.Title)) {
		return false
	}
	if f.Genre != "" && !slices.ContainsFunc(m.Genres, func(g string) bool { return strings.EqualFold(g, f.Genre) }) {
		return false
	}
	if f.Year != 0 && m.Year != f.Year {
		return false
	}
	return true
}

type MoviePage struct {
	Movies   []Movie
	Total    int64
	Page     int
	PageSize int
}
