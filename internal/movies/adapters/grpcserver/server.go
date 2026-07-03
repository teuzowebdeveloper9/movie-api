package grpcserver

import (
	"context"

	moviesv1 "github.com/teuzowebdeveloper9/movie-api/gen/movies/v1"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

type Server struct {
	moviesv1.UnimplementedMoviesServiceServer
	svc ports.MovieService
}

func New(svc ports.MovieService) *Server {
	return &Server{svc: svc}
}

func (s *Server) ListMovies(ctx context.Context, req *moviesv1.ListMoviesRequest) (*moviesv1.ListMoviesResponse, error) {
	page, err := s.svc.List(ctx, domain.ListFilter{
		Page:     int(req.GetPage()),
		PageSize: int(req.GetPageSize()),
		Title:    req.GetTitle(),
		Genre:    req.GetGenre(),
		Year:     int(req.GetYear()),
	})
	if err != nil {
		return nil, toStatusError(err)
	}

	movies := make([]*moviesv1.Movie, 0, len(page.Movies))
	for _, m := range page.Movies {
		movies = append(movies, toProto(m))
	}
	return &moviesv1.ListMoviesResponse{
		Movies:   movies,
		Total:    page.Total,
		Page:     int32(page.Page),
		PageSize: int32(page.PageSize),
	}, nil
}

func (s *Server) GetMovie(ctx context.Context, req *moviesv1.GetMovieRequest) (*moviesv1.GetMovieResponse, error) {
	movie, err := s.svc.Get(ctx, req.GetId())
	if err != nil {
		return nil, toStatusError(err)
	}
	return &moviesv1.GetMovieResponse{Movie: toProto(movie)}, nil
}

func (s *Server) CreateMovie(ctx context.Context, req *moviesv1.CreateMovieRequest) (*moviesv1.CreateMovieResponse, error) {
	movie, outcome, err := s.svc.Create(ctx, domain.NewMovieInput{
		Title:           req.GetTitle(),
		Year:            int(req.GetYear()),
		Cast:            req.GetCast(),
		Genres:          req.GetGenres(),
		Href:            req.GetHref(),
		Extract:         req.GetExtract(),
		Thumbnail:       req.GetThumbnail(),
		ThumbnailWidth:  int(req.GetThumbnailWidth()),
		ThumbnailHeight: int(req.GetThumbnailHeight()),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &moviesv1.CreateMovieResponse{
		Movie:  toProto(movie),
		Status: toProtoStatus(outcome),
	}, nil
}

func (s *Server) DeleteMovie(ctx context.Context, req *moviesv1.DeleteMovieRequest) (*moviesv1.DeleteMovieResponse, error) {
	outcome, err := s.svc.Delete(ctx, req.GetId())
	if err != nil {
		return nil, toStatusError(err)
	}
	return &moviesv1.DeleteMovieResponse{Status: toProtoStatus(outcome)}, nil
}
