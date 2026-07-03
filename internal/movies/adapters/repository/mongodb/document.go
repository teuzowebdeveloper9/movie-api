package mongodb

import (
	"time"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

type movieDocument struct {
	ID              string    `bson:"_id"`
	Title           string    `bson:"title"`
	Year            int       `bson:"year"`
	Cast            []string  `bson:"cast"`
	Genres          []string  `bson:"genres"`
	Href            string    `bson:"href"`
	Extract         string    `bson:"extract"`
	Thumbnail       string    `bson:"thumbnail"`
	ThumbnailWidth  int       `bson:"thumbnail_width"`
	ThumbnailHeight int       `bson:"thumbnail_height"`
	CreatedAt       time.Time `bson:"created_at"`
	UpdatedAt       time.Time `bson:"updated_at"`
}

func fromDomain(m domain.Movie) movieDocument {
	return movieDocument{
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
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func (d movieDocument) toDomain() domain.Movie {
	return domain.Movie{
		ID:              d.ID,
		Title:           d.Title,
		Year:            d.Year,
		Cast:            d.Cast,
		Genres:          d.Genres,
		Href:            d.Href,
		Extract:         d.Extract,
		Thumbnail:       d.Thumbnail,
		ThumbnailWidth:  d.ThumbnailWidth,
		ThumbnailHeight: d.ThumbnailHeight,
		CreatedAt:       d.CreatedAt.UTC(),
		UpdatedAt:       d.UpdatedAt.UTC(),
	}
}
