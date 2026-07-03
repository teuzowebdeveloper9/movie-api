package rabbitmq

import (
	"encoding/json"
	"time"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

const (
	Exchange    = "movies.events"
	DLXExchange = "movies.events.dlx"
	Queue       = "movies.write-requests"
	DLQueue     = "movies.write-requests.dlq"

	RoutingKeyCreateRequested = "movie.create.requested"
	RoutingKeyDeleteRequested = "movie.delete.requested"
)

type envelope struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

type moviePayload struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Year            int       `json:"year"`
	Cast            []string  `json:"cast"`
	Genres          []string  `json:"genres"`
	Href            string    `json:"href"`
	Extract         string    `json:"extract"`
	Thumbnail       string    `json:"thumbnail"`
	ThumbnailWidth  int       `json:"thumbnail_width"`
	ThumbnailHeight int       `json:"thumbnail_height"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type deletePayload struct {
	ID string `json:"id"`
}

func payloadFromDomain(m domain.Movie) moviePayload {
	return moviePayload{
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

func (p moviePayload) toDomain() domain.Movie {
	return domain.Movie{
		ID:              p.ID,
		Title:           p.Title,
		Year:            p.Year,
		Cast:            p.Cast,
		Genres:          p.Genres,
		Href:            p.Href,
		Extract:         p.Extract,
		Thumbnail:       p.Thumbnail,
		ThumbnailWidth:  p.ThumbnailWidth,
		ThumbnailHeight: p.ThumbnailHeight,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}
