package handler

import (
	"time"

	moviesv1 "github.com/teuzowebdeveloper9/movie-api/gen/movies/v1"
)

type CreateMovieRequest struct {
	Title           string   `json:"title" example:"The Matrix"`
	Year            int      `json:"year" example:"1999"`
	Cast            []string `json:"cast" example:"Keanu Reeves,Carrie-Anne Moss"`
	Genres          []string `json:"genres" example:"Action,Science Fiction"`
	Href            string   `json:"href" example:"The_Matrix"`
	Extract         string   `json:"extract" example:"The Matrix is a 1999 science fiction action film."`
	Thumbnail       string   `json:"thumbnail" example:"https://upload.wikimedia.org/wikipedia/en/c/c1/The_Matrix_Poster.jpg"`
	ThumbnailWidth  int      `json:"thumbnail_width" example:"220"`
	ThumbnailHeight int      `json:"thumbnail_height" example:"325"`
}

type MovieResponse struct {
	ID              string    `json:"id" example:"9a2cbe19-9c4d-4b41-8d5c-1c2f36bfb70c"`
	Title           string    `json:"title" example:"The Matrix"`
	Year            int       `json:"year" example:"1999"`
	Cast            []string  `json:"cast" example:"Keanu Reeves,Carrie-Anne Moss"`
	Genres          []string  `json:"genres" example:"Action,Science Fiction"`
	Href            string    `json:"href,omitempty" example:"The_Matrix"`
	Extract         string    `json:"extract,omitempty" example:"The Matrix is a 1999 science fiction action film."`
	Thumbnail       string    `json:"thumbnail,omitempty" example:"https://upload.wikimedia.org/wikipedia/en/c/c1/The_Matrix_Poster.jpg"`
	ThumbnailWidth  int       `json:"thumbnail_width,omitempty" example:"220"`
	ThumbnailHeight int       `json:"thumbnail_height,omitempty" example:"325"`
	CreatedAt       time.Time `json:"created_at" example:"2026-01-15T10:30:00Z"`
	UpdatedAt       time.Time `json:"updated_at" example:"2026-01-15T10:30:00Z"`
}

type PageMeta struct {
	Page       int   `json:"page" example:"1"`
	PageSize   int   `json:"page_size" example:"20"`
	Total      int64 `json:"total" example:"20"`
	TotalPages int   `json:"total_pages" example:"1"`
}

type MovieListResponse struct {
	Data []MovieResponse `json:"data"`
	Meta PageMeta        `json:"meta"`
}

type AcceptedResponse struct {
	ID      string `json:"id" example:"9a2cbe19-9c4d-4b41-8d5c-1c2f36bfb70c"`
	Status  string `json:"status" example:"accepted"`
	Message string `json:"message" example:"request accepted for asynchronous processing"`
}

type ErrorDetail struct {
	Code    string `json:"code" example:"not_found"`
	Message string `json:"message" example:"movie not found"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

func movieFromProto(m *moviesv1.Movie) MovieResponse {
	return MovieResponse{
		ID:              m.GetId(),
		Title:           m.GetTitle(),
		Year:            int(m.GetYear()),
		Cast:            m.GetCast(),
		Genres:          m.GetGenres(),
		Href:            m.GetHref(),
		Extract:         m.GetExtract(),
		Thumbnail:       m.GetThumbnail(),
		ThumbnailWidth:  int(m.GetThumbnailWidth()),
		ThumbnailHeight: int(m.GetThumbnailHeight()),
		CreatedAt:       m.GetCreatedAt().AsTime(),
		UpdatedAt:       m.GetUpdatedAt().AsTime(),
	}
}
