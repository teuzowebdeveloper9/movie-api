//go:build integration

package mongodb_test

import (
	"context"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmongodb "github.com/testcontainers/testcontainers-go/modules/mongodb"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/mongodb"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/repositorytest"
)

// mongoImage matches docker-compose.yml and deploy/k8s/mongodb.yaml so the
// suite runs against the same MongoDB the application ships with.
const mongoImage = "mongo:7"

// dockerInit runs docker-init (tini) as the container's PID 1 so signals are
// forwarded and zombies reaped — without it, some hosts cannot stop the
// container at cleanup ("PID ... is zombie and can not be killed").
func dockerInit() testcontainers.CustomizeRequestOption {
	return testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
		enabled := true
		hc.Init = &enabled
	})
}

func TestRepositoryIntegration(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcmongodb.Run(ctx, mongoImage, dockerInit())
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	client, db, err := mongodb.Connect(ctx, uri, "movies_integration")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	repo := mongodb.NewRepository(db)
	require.NoError(t, repo.EnsureIndexes(ctx))

	repositorytest.Run(t, repo)
}
