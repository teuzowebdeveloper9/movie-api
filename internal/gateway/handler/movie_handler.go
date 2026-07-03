package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	moviesv1 "github.com/teuzowebdeveloper9/movie-api/gen/movies/v1"
)

const maxBodySize = 1 << 20

type MovieHandler struct {
	client moviesv1.MoviesServiceClient
	logger *slog.Logger
}

func NewMovieHandler(client moviesv1.MoviesServiceClient, logger *slog.Logger) *MovieHandler {
	return &MovieHandler{client: client, logger: logger}
}

// @Summary		List movies
// @Description	Returns a paginated list of movies. Supports case-insensitive title search plus genre and year filters.
// @Tags			movies
// @Produce		json
// @Param			page		query		int		false	"1-based page number"			default(1)
// @Param			page_size	query		int		false	"page size (max 100)"			default(20)
// @Param			title		query		string	false	"case-insensitive title search"	example(matrix)
// @Param			genre		query		string	false	"exact genre filter"			example(Drama)
// @Param			year		query		int		false	"release year filter"			example(1999)
// @Success		200			{object}	MovieListResponse
// @Failure		400			{object}	ErrorResponse
// @Failure		503			{object}	ErrorResponse
// @Router			/movies [get]
func (h *MovieHandler) List(w http.ResponseWriter, r *http.Request) {
	page, err := intQuery(r, "page")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	pageSize, err := intQuery(r, "page_size")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	year, err := intQuery(r, "year")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	resp, err := h.client.ListMovies(r.Context(), &moviesv1.ListMoviesRequest{
		Page:     int32(page),
		PageSize: int32(pageSize),
		Title:    r.URL.Query().Get("title"),
		Genre:    r.URL.Query().Get("genre"),
		Year:     int32(year),
	})
	if err != nil {
		writeGRPCError(w, r, h.logger, err)
		return
	}

	movies := make([]MovieResponse, 0, len(resp.GetMovies()))
	for _, m := range resp.GetMovies() {
		movies = append(movies, movieFromProto(m))
	}
	writeJSON(w, http.StatusOK, MovieListResponse{
		Data: movies,
		Meta: PageMeta{
			Page:       int(resp.GetPage()),
			PageSize:   int(resp.GetPageSize()),
			Total:      resp.GetTotal(),
			TotalPages: totalPages(resp.GetTotal(), int(resp.GetPageSize())),
		},
	})
}

// @Summary		Get a movie by ID
// @Description	Returns a single movie.
// @Tags			movies
// @Produce		json
// @Param			id	path		string	true	"movie ID"
// @Success		200	{object}	MovieResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		503	{object}	ErrorResponse
// @Router			/movies/{id} [get]
func (h *MovieHandler) Get(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.GetMovie(r.Context(), &moviesv1.GetMovieRequest{Id: chi.URLParam(r, "id")})
	if err != nil {
		writeGRPCError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, movieFromProto(resp.GetMovie()))
}

// @Summary		Create a movie
// @Description	Registers a new movie. In event-driven mode the write is queued and the endpoint answers 202 Accepted; otherwise it answers 201 Created.
// @Tags			movies
// @Accept			json
// @Produce		json
// @Param			movie	body		CreateMovieRequest	true	"movie to create"
// @Success		201		{object}	MovieResponse		"created synchronously"
// @Success		202		{object}	AcceptedResponse	"accepted for asynchronous processing"
// @Failure		400		{object}	ErrorResponse
// @Failure		503		{object}	ErrorResponse
// @Header			201,202	{string}	Location	"URL of the (future) resource"
// @Router			/movies [post]
func (h *MovieHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body CreateMovieRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	resp, err := h.client.CreateMovie(r.Context(), &moviesv1.CreateMovieRequest{
		Title:           body.Title,
		Year:            int32(body.Year),
		Cast:            body.Cast,
		Genres:          body.Genres,
		Href:            body.Href,
		Extract:         body.Extract,
		Thumbnail:       body.Thumbnail,
		ThumbnailWidth:  int32(body.ThumbnailWidth),
		ThumbnailHeight: int32(body.ThumbnailHeight),
	})
	if err != nil {
		writeGRPCError(w, r, h.logger, err)
		return
	}

	movie := resp.GetMovie()
	w.Header().Set("Location", "/movies/"+movie.GetId())
	if resp.GetStatus() == moviesv1.OperationStatus_OPERATION_STATUS_ACCEPTED {
		writeJSON(w, http.StatusAccepted, AcceptedResponse{
			ID:      movie.GetId(),
			Status:  "accepted",
			Message: "movie creation accepted for asynchronous processing",
		})
		return
	}
	writeJSON(w, http.StatusCreated, movieFromProto(movie))
}

// @Summary		Delete a movie
// @Description	Removes a movie by ID. In event-driven mode the delete is queued and the endpoint answers 202 Accepted; otherwise it answers 204 No Content.
// @Tags			movies
// @Produce		json
// @Param			id	path	string	true	"movie ID"
// @Success		202	{object}	AcceptedResponse	"accepted for asynchronous processing"
// @Success		204	"deleted synchronously"
// @Failure		404	{object}	ErrorResponse
// @Failure		503	{object}	ErrorResponse
// @Router			/movies/{id} [delete]
func (h *MovieHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resp, err := h.client.DeleteMovie(r.Context(), &moviesv1.DeleteMovieRequest{Id: id})
	if err != nil {
		writeGRPCError(w, r, h.logger, err)
		return
	}

	if resp.GetStatus() == moviesv1.OperationStatus_OPERATION_STATUS_ACCEPTED {
		writeJSON(w, http.StatusAccepted, AcceptedResponse{
			ID:      id,
			Status:  "accepted",
			Message: "movie deletion accepted for asynchronous processing",
		})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON body: unexpected trailing data")
	}
	return nil
}

func intQuery(r *http.Request, key string) (int, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("query parameter %q must be an integer", key)
	}
	return value, nil
}

func totalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}
