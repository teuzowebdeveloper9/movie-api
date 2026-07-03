package grpcserver

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	moviesv1 "github.com/teuzowebdeveloper9/movie-api/gen/movies/v1"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

func toProto(m domain.Movie) *moviesv1.Movie {
	return &moviesv1.Movie{
		Id:              m.ID,
		Title:           m.Title,
		Year:            int32(m.Year),
		Cast:            m.Cast,
		Genres:          m.Genres,
		Href:            m.Href,
		Extract:         m.Extract,
		Thumbnail:       m.Thumbnail,
		ThumbnailWidth:  int32(m.ThumbnailWidth),
		ThumbnailHeight: int32(m.ThumbnailHeight),
		CreatedAt:       timestamppb.New(m.CreatedAt),
		UpdatedAt:       timestamppb.New(m.UpdatedAt),
	}
}

func toProtoStatus(outcome ports.WriteOutcome) moviesv1.OperationStatus {
	switch outcome {
	case ports.WriteCompleted:
		return moviesv1.OperationStatus_OPERATION_STATUS_COMPLETED
	case ports.WriteAccepted:
		return moviesv1.OperationStatus_OPERATION_STATUS_ACCEPTED
	default:
		return moviesv1.OperationStatus_OPERATION_STATUS_UNSPECIFIED
	}
}

func toStatusError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "movie not found")
	case errors.Is(err, domain.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "movie already exists")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request timed out")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request cancelled")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
