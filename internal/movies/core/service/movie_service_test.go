package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/service"
)

type repositoryMock struct{ mock.Mock }

func (m *repositoryMock) List(ctx context.Context, filter domain.ListFilter) (domain.MoviePage, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(domain.MoviePage), args.Error(1)
}

func (m *repositoryMock) GetByID(ctx context.Context, id string) (domain.Movie, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(domain.Movie), args.Error(1)
}

func (m *repositoryMock) Create(ctx context.Context, movie domain.Movie) error {
	return m.Called(ctx, movie).Error(0)
}

func (m *repositoryMock) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

type publisherMock struct{ mock.Mock }

func (m *publisherMock) MovieCreateRequested(ctx context.Context, movie domain.Movie) error {
	return m.Called(ctx, movie).Error(0)
}

func (m *publisherMock) MovieDeleteRequested(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

var (
	fixedTime = time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	fixedID   = "movie-123"
)

func newService(repo ports.MovieRepository, extra ...service.Option) *service.MovieService {
	opts := append([]service.Option{
		service.WithClock(func() time.Time { return fixedTime }),
		service.WithIDGenerator(func() string { return fixedID }),
	}, extra...)
	return service.New(repo, opts...)
}

func validInput() domain.NewMovieInput {
	return domain.NewMovieInput{
		Title:  "The Matrix",
		Year:   1999,
		Cast:   []string{"Keanu Reeves"},
		Genres: []string{"Action"},
	}
}

func TestCreate_SyncPersistsAndReturnsCompleted(t *testing.T) {
	repo := new(repositoryMock)
	repo.On("Create", mock.Anything, mock.MatchedBy(func(m domain.Movie) bool {
		return m.ID == fixedID && m.Title == "The Matrix" && m.CreatedAt.Equal(fixedTime)
	})).Return(nil)

	movie, outcome, err := newService(repo).Create(context.Background(), validInput())

	require.NoError(t, err)
	assert.Equal(t, ports.WriteCompleted, outcome)
	assert.Equal(t, fixedID, movie.ID)
	repo.AssertExpectations(t)
}

func TestCreate_AsyncPublishesInsteadOfPersisting(t *testing.T) {
	repo := new(repositoryMock)
	publisher := new(publisherMock)
	publisher.On("MovieCreateRequested", mock.Anything, mock.MatchedBy(func(m domain.Movie) bool {
		return m.ID == fixedID && m.Title == "The Matrix"
	})).Return(nil)

	svc := newService(repo, service.WithPublisher(publisher))
	movie, outcome, err := svc.Create(context.Background(), validInput())

	require.NoError(t, err)
	assert.Equal(t, ports.WriteAccepted, outcome)
	assert.Equal(t, fixedID, movie.ID)
	publisher.AssertExpectations(t)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestCreate_InvalidInputFailsBeforeAnySideEffect(t *testing.T) {
	repo := new(repositoryMock)
	publisher := new(publisherMock)

	svc := newService(repo, service.WithPublisher(publisher))
	_, _, err := svc.Create(context.Background(), domain.NewMovieInput{Title: " ", Year: 0})

	assert.ErrorIs(t, err, domain.ErrInvalid)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	publisher.AssertNotCalled(t, "MovieCreateRequested", mock.Anything, mock.Anything)
}

func TestCreate_PublisherFailurePropagates(t *testing.T) {
	repo := new(repositoryMock)
	publisher := new(publisherMock)
	publisher.On("MovieCreateRequested", mock.Anything, mock.Anything).Return(errors.New("broker down"))

	svc := newService(repo, service.WithPublisher(publisher))
	_, _, err := svc.Create(context.Background(), validInput())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "broker down")
}

func TestCreate_RepositoryFailurePropagates(t *testing.T) {
	repo := new(repositoryMock)
	repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("db down"))

	_, _, err := newService(repo).Create(context.Background(), validInput())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestGet_BlankIDIsInvalid(t *testing.T) {
	repo := new(repositoryMock)

	_, err := newService(repo).Get(context.Background(), "   ")

	assert.ErrorIs(t, err, domain.ErrInvalid)
	repo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
}

func TestGet_TrimsIDAndDelegates(t *testing.T) {
	repo := new(repositoryMock)
	stored := domain.Movie{ID: "movie-1", Title: "Se7en", Year: 1995}
	repo.On("GetByID", mock.Anything, "movie-1").Return(stored, nil)

	movie, err := newService(repo).Get(context.Background(), " movie-1 ")

	require.NoError(t, err)
	assert.Equal(t, stored, movie)
	repo.AssertExpectations(t)
}

func TestGet_NotFoundPropagates(t *testing.T) {
	repo := new(repositoryMock)
	repo.On("GetByID", mock.Anything, "ghost").Return(domain.Movie{}, domain.ErrNotFound)

	_, err := newService(repo).Get(context.Background(), "ghost")

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestList_NormalizesFilterBeforeQuerying(t *testing.T) {
	repo := new(repositoryMock)
	expected := domain.ListFilter{Page: 1, PageSize: domain.DefaultPageSize, Title: "matrix"}
	repo.On("List", mock.Anything, expected).Return(domain.MoviePage{Page: 1, PageSize: domain.DefaultPageSize}, nil)

	_, err := newService(repo).List(context.Background(), domain.ListFilter{Page: -1, PageSize: 0, Title: " matrix "})

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestDelete_SyncRemovesAndReturnsCompleted(t *testing.T) {
	repo := new(repositoryMock)
	repo.On("GetByID", mock.Anything, "movie-1").Return(domain.Movie{ID: "movie-1"}, nil)
	repo.On("Delete", mock.Anything, "movie-1").Return(nil)

	outcome, err := newService(repo).Delete(context.Background(), "movie-1")

	require.NoError(t, err)
	assert.Equal(t, ports.WriteCompleted, outcome)
	repo.AssertExpectations(t)
}

func TestDelete_AsyncPublishesInsteadOfDeleting(t *testing.T) {
	repo := new(repositoryMock)
	publisher := new(publisherMock)
	repo.On("GetByID", mock.Anything, "movie-1").Return(domain.Movie{ID: "movie-1"}, nil)
	publisher.On("MovieDeleteRequested", mock.Anything, "movie-1").Return(nil)

	svc := newService(repo, service.WithPublisher(publisher))
	outcome, err := svc.Delete(context.Background(), "movie-1")

	require.NoError(t, err)
	assert.Equal(t, ports.WriteAccepted, outcome)
	publisher.AssertExpectations(t)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestDelete_MissingMovieFailsFast(t *testing.T) {
	repo := new(repositoryMock)
	publisher := new(publisherMock)
	repo.On("GetByID", mock.Anything, "ghost").Return(domain.Movie{}, domain.ErrNotFound)

	svc := newService(repo, service.WithPublisher(publisher))
	_, err := svc.Delete(context.Background(), "ghost")

	assert.ErrorIs(t, err, domain.ErrNotFound)
	publisher.AssertNotCalled(t, "MovieDeleteRequested", mock.Anything, mock.Anything)
}

func TestApplyCreate_PersistsValidMovie(t *testing.T) {
	repo := new(repositoryMock)
	movie, err := domain.NewMovie("movie-1", validInput(), fixedTime)
	require.NoError(t, err)
	repo.On("Create", mock.Anything, movie).Return(nil)

	require.NoError(t, newService(repo).ApplyCreate(context.Background(), movie))
	repo.AssertExpectations(t)
}

func TestApplyCreate_DuplicateIsIdempotent(t *testing.T) {
	repo := new(repositoryMock)
	movie, err := domain.NewMovie("movie-1", validInput(), fixedTime)
	require.NoError(t, err)
	repo.On("Create", mock.Anything, movie).Return(domain.ErrAlreadyExists)

	assert.NoError(t, newService(repo).ApplyCreate(context.Background(), movie))
}

func TestApplyCreate_RejectsInvalidEvent(t *testing.T) {
	repo := new(repositoryMock)

	err := newService(repo).ApplyCreate(context.Background(), domain.Movie{ID: "movie-1"})

	assert.ErrorIs(t, err, domain.ErrInvalid)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestApplyDelete_MissingIsIdempotent(t *testing.T) {
	repo := new(repositoryMock)
	repo.On("Delete", mock.Anything, "ghost").Return(domain.ErrNotFound)

	assert.NoError(t, newService(repo).ApplyDelete(context.Background(), "ghost"))
}

func TestApplyDelete_TransientFailurePropagates(t *testing.T) {
	repo := new(repositoryMock)
	repo.On("Delete", mock.Anything, "movie-1").Return(errors.New("db down"))

	err := newService(repo).ApplyDelete(context.Background(), "movie-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
