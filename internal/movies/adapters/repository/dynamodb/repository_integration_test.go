//go:build integration

package dynamodb_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/dynamodb"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/repositorytest"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
)

// localstackImage matches docker-compose.localstack.yml so the suite runs
// against the same emulator the application ships with.
const localstackImage = "localstack/localstack:3"

// dockerInit runs docker-init (tini) as the container's PID 1 so signals are
// forwarded and zombies reaped — without it, some hosts cannot stop the
// container at cleanup ("PID ... is zombie and can not be killed").
func dockerInit() testcontainers.CustomizeRequestOption {
	return testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
		enabled := true
		hc.Init = &enabled
	})
}

func startLocalStack(t *testing.T) *awsdynamodb.Client {
	t.Helper()
	ctx := context.Background()

	ctr, err := localstack.Run(ctx, localstackImage,
		testcontainers.WithEnv(map[string]string{
			"SERVICES":              "dynamodb",
			"EAGER_SERVICE_LOADING": "1",
			"DYNAMODB_IN_MEMORY":    "1",
		}),
		dockerInit())
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	host, err := ctr.Host(ctx)
	require.NoError(t, err)
	port, err := ctr.MappedPort(ctx, "4566/tcp")
	require.NoError(t, err)

	client, err := dynamodb.NewClient(ctx, dynamodb.Config{
		Endpoint: fmt.Sprintf("http://%s:%s", host, port.Port()),
		Region:   "us-east-1",
	})
	require.NoError(t, err)
	return client
}

func TestRepositoryIntegration(t *testing.T) {
	ctx := context.Background()
	client := startLocalStack(t)

	repo := dynamodb.NewRepository(client, "movies_integration")
	require.NoError(t, repo.EnsureTable(ctx))
	// idempotent: a second call on a ready table must be a no-op
	require.NoError(t, repo.EnsureTable(ctx))

	repositorytest.Run(t, repo)
}

// TestEnsureTableBackfillIntegration exercises the upgrade path: a table
// created before the title-sort GSI existed (id key only, items without the
// derived attributes) must gain the index and have its items backfilled, or
// they would stay invisible to List (sparse GSI).
func TestEnsureTableBackfillIntegration(t *testing.T) {
	ctx := context.Background()
	client := startLocalStack(t)
	const table = "movies_legacy"

	_, err := client.CreateTable(ctx, &awsdynamodb.CreateTableInput{
		TableName:   aws.String(table),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
		},
	})
	require.NoError(t, err)
	waiter := awsdynamodb.NewTableExistsWaiter(client)
	require.NoError(t, waiter.Wait(ctx,
		&awsdynamodb.DescribeTableInput{TableName: aws.String(table)}, 60*time.Second))

	created := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for id, title := range map[string]string{"42": "Metropolis", "43": "Nosferatu"} {
		_, err = client.PutItem(ctx, &awsdynamodb.PutItemInput{
			TableName: aws.String(table),
			Item: map[string]types.AttributeValue{
				"id":         &types.AttributeValueMemberS{Value: id},
				"title":      &types.AttributeValueMemberS{Value: title},
				"year":       &types.AttributeValueMemberN{Value: "1927"},
				"created_at": &types.AttributeValueMemberS{Value: created},
				"updated_at": &types.AttributeValueMemberS{Value: created},
			},
		})
		require.NoError(t, err)
	}

	repo := dynamodb.NewRepository(client, table)
	require.NoError(t, repo.EnsureTable(ctx))

	page, err := repo.List(ctx, domain.ListFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), page.Total)
	require.Len(t, page.Movies, 2)
	assert.Equal(t, "Metropolis", page.Movies[0].Title)
	assert.Equal(t, "Nosferatu", page.Movies[1].Title)
}
