package grpcserver_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	moviesv1 "github.com/teuzowebdeveloper9/movie-api/gen/movies/v1"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/grpcserver"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/memory"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/service"
)

func newTestClient(t *testing.T) moviesv1.MoviesServiceClient {
	t.Helper()

	svc := service.New(memory.New())
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	moviesv1.RegisterMoviesServiceServer(server, grpcserver.New(svc))

	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return moviesv1.NewMoviesServiceClient(conn)
}

func createMovie(t *testing.T, client moviesv1.MoviesServiceClient, title string, year int32, genres ...string) *moviesv1.Movie {
	t.Helper()
	resp, err := client.CreateMovie(context.Background(), &moviesv1.CreateMovieRequest{
		Title:  title,
		Year:   year,
		Cast:   []string{"Someone"},
		Genres: genres,
	})
	require.NoError(t, err)
	return resp.GetMovie()
}

func TestFullCRUDFlow(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)

	created := createMovie(t, client, "The Matrix", 1999, "Action")
	assert.NotEmpty(t, created.GetId())

	second, err := client.CreateMovie(ctx, &moviesv1.CreateMovieRequest{Title: "Inception", Year: 2010})
	require.NoError(t, err)
	assert.Equal(t, moviesv1.OperationStatus_OPERATION_STATUS_COMPLETED, second.GetStatus())

	got, err := client.GetMovie(ctx, &moviesv1.GetMovieRequest{Id: created.GetId()})
	require.NoError(t, err)
	assert.Equal(t, "The Matrix", got.GetMovie().GetTitle())
	assert.Equal(t, int32(1999), got.GetMovie().GetYear())

	list, err := client.ListMovies(ctx, &moviesv1.ListMoviesRequest{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, list.GetTotal())

	deleted, err := client.DeleteMovie(ctx, &moviesv1.DeleteMovieRequest{Id: created.GetId()})
	require.NoError(t, err)
	assert.Equal(t, moviesv1.OperationStatus_OPERATION_STATUS_COMPLETED, deleted.GetStatus())

	_, err = client.GetMovie(ctx, &moviesv1.GetMovieRequest{Id: created.GetId()})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestCreateMovie_InvalidReturnsInvalidArgument(t *testing.T) {
	client := newTestClient(t)

	_, err := client.CreateMovie(context.Background(), &moviesv1.CreateMovieRequest{Title: " ", Year: 1})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "title")
	assert.Contains(t, st.Message(), "year")
}

func TestGetMovie_MissingReturnsNotFound(t *testing.T) {
	client := newTestClient(t)

	_, err := client.GetMovie(context.Background(), &moviesv1.GetMovieRequest{Id: "ghost"})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestDeleteMovie_MissingReturnsNotFound(t *testing.T) {
	client := newTestClient(t)

	_, err := client.DeleteMovie(context.Background(), &moviesv1.DeleteMovieRequest{Id: "ghost"})

	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestListMovies_FiltersAndPaginates(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	createMovie(t, client, "Se7en", 1995, "Crime")
	createMovie(t, client, "The Matrix", 1999, "Action")
	createMovie(t, client, "Inception", 2010, "Action")

	byGenre, err := client.ListMovies(ctx, &moviesv1.ListMoviesRequest{Genre: "Action"})
	require.NoError(t, err)
	assert.EqualValues(t, 2, byGenre.GetTotal())

	byTitle, err := client.ListMovies(ctx, &moviesv1.ListMoviesRequest{Title: "matrix"})
	require.NoError(t, err)
	require.Len(t, byTitle.GetMovies(), 1)
	assert.Equal(t, "The Matrix", byTitle.GetMovies()[0].GetTitle())

	paged, err := client.ListMovies(ctx, &moviesv1.ListMoviesRequest{Page: 2, PageSize: 2})
	require.NoError(t, err)
	assert.EqualValues(t, 3, paged.GetTotal())
	assert.Len(t, paged.GetMovies(), 1)
	assert.EqualValues(t, 2, paged.GetPage())
}
