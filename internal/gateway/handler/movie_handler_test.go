package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	moviesv1 "github.com/teuzowebdeveloper9/movie-api/gen/movies/v1"
	"github.com/teuzowebdeveloper9/movie-api/internal/gateway/config"
	"github.com/teuzowebdeveloper9/movie-api/internal/gateway/handler"
	"github.com/teuzowebdeveloper9/movie-api/internal/gateway/router"
)

type clientMock struct {
	listFn   func(ctx context.Context, in *moviesv1.ListMoviesRequest, opts ...grpc.CallOption) (*moviesv1.ListMoviesResponse, error)
	getFn    func(ctx context.Context, in *moviesv1.GetMovieRequest, opts ...grpc.CallOption) (*moviesv1.GetMovieResponse, error)
	createFn func(ctx context.Context, in *moviesv1.CreateMovieRequest, opts ...grpc.CallOption) (*moviesv1.CreateMovieResponse, error)
	deleteFn func(ctx context.Context, in *moviesv1.DeleteMovieRequest, opts ...grpc.CallOption) (*moviesv1.DeleteMovieResponse, error)
}

func (c *clientMock) ListMovies(ctx context.Context, in *moviesv1.ListMoviesRequest, opts ...grpc.CallOption) (*moviesv1.ListMoviesResponse, error) {
	return c.listFn(ctx, in, opts...)
}

func (c *clientMock) GetMovie(ctx context.Context, in *moviesv1.GetMovieRequest, opts ...grpc.CallOption) (*moviesv1.GetMovieResponse, error) {
	return c.getFn(ctx, in, opts...)
}

func (c *clientMock) CreateMovie(ctx context.Context, in *moviesv1.CreateMovieRequest, opts ...grpc.CallOption) (*moviesv1.CreateMovieResponse, error) {
	return c.createFn(ctx, in, opts...)
}

func (c *clientMock) DeleteMovie(ctx context.Context, in *moviesv1.DeleteMovieRequest, opts ...grpc.CallOption) (*moviesv1.DeleteMovieResponse, error) {
	return c.deleteFn(ctx, in, opts...)
}

func newRouter(client moviesv1.MoviesServiceClient) http.Handler {
	cfg := config.Config{
		Env:            "test",
		SwaggerEnabled: false,
		RequestTimeout: 5 * time.Second,
		RateLimitRPM:   10000,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := handler.NewMovieHandler(client, logger)
	return router.New(cfg, h, logger, func(context.Context) error { return nil })
}

func protoMovie(id, title string, year int32) *moviesv1.Movie {
	ts := timestamppb.New(time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC))
	return &moviesv1.Movie{
		Id:        id,
		Title:     title,
		Year:      year,
		Cast:      []string{"Someone"},
		Genres:    []string{"Action"},
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}

func doRequest(t *testing.T, h http.Handler, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestListMovies_ReturnsPageWithMeta(t *testing.T) {
	client := &clientMock{
		listFn: func(_ context.Context, in *moviesv1.ListMoviesRequest, _ ...grpc.CallOption) (*moviesv1.ListMoviesResponse, error) {
			assert.EqualValues(t, 2, in.GetPage())
			assert.EqualValues(t, 10, in.GetPageSize())
			assert.Equal(t, "matrix", in.GetTitle())
			return &moviesv1.ListMoviesResponse{
				Movies:   []*moviesv1.Movie{protoMovie("movie-1", "The Matrix", 1999)},
				Total:    11,
				Page:     2,
				PageSize: 10,
			}, nil
		},
	}

	rec := doRequest(t, newRouter(client), http.MethodGet, "/movies?page=2&page_size=10&title=matrix", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var body handler.MovieListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "The Matrix", body.Data[0].Title)
	assert.EqualValues(t, 11, body.Meta.Total)
	assert.Equal(t, 2, body.Meta.TotalPages)
}

func TestListMovies_RejectsNonIntegerQuery(t *testing.T) {
	rec := doRequest(t, newRouter(&clientMock{}), http.MethodGet, "/movies?page=abc", nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body handler.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "invalid_request", body.Error.Code)
}

func TestGetMovie_Found(t *testing.T) {
	client := &clientMock{
		getFn: func(_ context.Context, in *moviesv1.GetMovieRequest, _ ...grpc.CallOption) (*moviesv1.GetMovieResponse, error) {
			assert.Equal(t, "movie-1", in.GetId())
			return &moviesv1.GetMovieResponse{Movie: protoMovie("movie-1", "The Matrix", 1999)}, nil
		},
	}

	rec := doRequest(t, newRouter(client), http.MethodGet, "/movies/movie-1", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var body handler.MovieResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "movie-1", body.ID)
}

func TestGetMovie_NotFoundMapsTo404(t *testing.T) {
	client := &clientMock{
		getFn: func(context.Context, *moviesv1.GetMovieRequest, ...grpc.CallOption) (*moviesv1.GetMovieResponse, error) {
			return nil, status.Error(codes.NotFound, "movie not found")
		},
	}

	rec := doRequest(t, newRouter(client), http.MethodGet, "/movies/ghost", nil)

	require.Equal(t, http.StatusNotFound, rec.Code)
	var body handler.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "not_found", body.Error.Code)
	assert.Equal(t, "movie not found", body.Error.Message)
}

func TestCreateMovie_SyncReturns201WithLocation(t *testing.T) {
	client := &clientMock{
		createFn: func(_ context.Context, in *moviesv1.CreateMovieRequest, _ ...grpc.CallOption) (*moviesv1.CreateMovieResponse, error) {
			assert.Equal(t, "The Matrix", in.GetTitle())
			return &moviesv1.CreateMovieResponse{
				Movie:  protoMovie("movie-1", in.GetTitle(), in.GetYear()),
				Status: moviesv1.OperationStatus_OPERATION_STATUS_COMPLETED,
			}, nil
		},
	}
	payload := []byte(`{"title":"The Matrix","year":1999,"cast":["Keanu Reeves"],"genres":["Action"]}`)

	rec := doRequest(t, newRouter(client), http.MethodPost, "/movies", payload)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "/movies/movie-1", rec.Header().Get("Location"))
	var body handler.MovieResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "movie-1", body.ID)
}

func TestCreateMovie_AsyncReturns202(t *testing.T) {
	client := &clientMock{
		createFn: func(_ context.Context, in *moviesv1.CreateMovieRequest, _ ...grpc.CallOption) (*moviesv1.CreateMovieResponse, error) {
			return &moviesv1.CreateMovieResponse{
				Movie:  protoMovie("movie-1", in.GetTitle(), in.GetYear()),
				Status: moviesv1.OperationStatus_OPERATION_STATUS_ACCEPTED,
			}, nil
		},
	}
	payload := []byte(`{"title":"The Matrix","year":1999}`)

	rec := doRequest(t, newRouter(client), http.MethodPost, "/movies", payload)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "/movies/movie-1", rec.Header().Get("Location"))
	var body handler.AcceptedResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "accepted", body.Status)
	assert.Equal(t, "movie-1", body.ID)
}

func TestCreateMovie_MalformedJSONReturns400(t *testing.T) {
	rec := doRequest(t, newRouter(&clientMock{}), http.MethodPost, "/movies", []byte(`{"title":`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body handler.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "invalid_body", body.Error.Code)
}

func TestCreateMovie_UnknownFieldReturns400(t *testing.T) {
	rec := doRequest(t, newRouter(&clientMock{}), http.MethodPost, "/movies", []byte(`{"title":"x","hacker_field":true}`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateMovie_ValidationErrorMapsTo400(t *testing.T) {
	client := &clientMock{
		createFn: func(context.Context, *moviesv1.CreateMovieRequest, ...grpc.CallOption) (*moviesv1.CreateMovieResponse, error) {
			return nil, status.Error(codes.InvalidArgument, "invalid movie: title must not be empty")
		},
	}

	rec := doRequest(t, newRouter(client), http.MethodPost, "/movies", []byte(`{"title":""}`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body handler.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body.Error.Message, "title")
}

func TestDeleteMovie_SyncReturns204(t *testing.T) {
	client := &clientMock{
		deleteFn: func(_ context.Context, in *moviesv1.DeleteMovieRequest, _ ...grpc.CallOption) (*moviesv1.DeleteMovieResponse, error) {
			assert.Equal(t, "movie-1", in.GetId())
			return &moviesv1.DeleteMovieResponse{Status: moviesv1.OperationStatus_OPERATION_STATUS_COMPLETED}, nil
		},
	}

	rec := doRequest(t, newRouter(client), http.MethodDelete, "/movies/movie-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestDeleteMovie_AsyncReturns202(t *testing.T) {
	client := &clientMock{
		deleteFn: func(context.Context, *moviesv1.DeleteMovieRequest, ...grpc.CallOption) (*moviesv1.DeleteMovieResponse, error) {
			return &moviesv1.DeleteMovieResponse{Status: moviesv1.OperationStatus_OPERATION_STATUS_ACCEPTED}, nil
		},
	}

	rec := doRequest(t, newRouter(client), http.MethodDelete, "/movies/movie-1", nil)

	require.Equal(t, http.StatusAccepted, rec.Code)
	var body handler.AcceptedResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "movie-1", body.ID)
}

func TestUpstreamUnavailableMapsTo503(t *testing.T) {
	client := &clientMock{
		listFn: func(context.Context, *moviesv1.ListMoviesRequest, ...grpc.CallOption) (*moviesv1.ListMoviesResponse, error) {
			return nil, status.Error(codes.Unavailable, "connection refused")
		},
	}

	rec := doRequest(t, newRouter(client), http.MethodGet, "/movies", nil)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	var body handler.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "upstream_unavailable", body.Error.Code)
	assert.NotContains(t, body.Error.Message, "connection refused")
}

func TestUnknownRouteReturnsJSON404(t *testing.T) {
	rec := doRequest(t, newRouter(&clientMock{}), http.MethodGet, "/unknown", nil)

	require.Equal(t, http.StatusNotFound, rec.Code)
	var body handler.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "route_not_found", body.Error.Code)
}

func TestSwaggerDisabledRouteIsAbsent(t *testing.T) {
	rec := doRequest(t, newRouter(&clientMock{}), http.MethodGet, "/swagger/index.html", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
