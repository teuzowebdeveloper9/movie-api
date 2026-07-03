package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

type Config struct {
	Endpoint string
	Region   string
	Table    string
}

func NewClient(ctx context.Context, cfg Config) (*awsdynamodb.Client, error) {
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.Endpoint != "" {
		loadOpts = append(loadOpts,
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("loading aws config: %w", err)
	}
	return awsdynamodb.NewFromConfig(awsCfg, func(o *awsdynamodb.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	}), nil
}

type Repository struct {
	client *awsdynamodb.Client
	table  string
}

var _ ports.MovieRepository = (*Repository)(nil)

func NewRepository(client *awsdynamodb.Client, table string) *Repository {
	return &Repository{client: client, table: table}
}

func (r *Repository) EnsureTable(ctx context.Context) error {
	_, err := r.client.DescribeTable(ctx, &awsdynamodb.DescribeTableInput{TableName: aws.String(r.table)})
	if err == nil {
		return nil
	}
	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return fmt.Errorf("describing table %q: %w", r.table, err)
	}

	_, err = r.client.CreateTable(ctx, &awsdynamodb.CreateTableInput{
		TableName:   aws.String(r.table),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
		},
	})
	if err != nil {
		return fmt.Errorf("creating table %q: %w", r.table, err)
	}

	waiter := awsdynamodb.NewTableExistsWaiter(r.client)
	if err := waiter.Wait(ctx, &awsdynamodb.DescribeTableInput{TableName: aws.String(r.table)}, 60*time.Second); err != nil {
		return fmt.Errorf("waiting for table %q: %w", r.table, err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter domain.ListFilter) (domain.MoviePage, error) {
	filter = filter.Normalized()

	var matched []domain.Movie
	paginator := awsdynamodb.NewScanPaginator(r.client, &awsdynamodb.ScanInput{TableName: aws.String(r.table)})
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return domain.MoviePage{}, fmt.Errorf("scanning movies: %w", err)
		}
		var items []movieItem
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
			return domain.MoviePage{}, fmt.Errorf("decoding movies: %w", err)
		}
		for _, it := range items {
			movie, err := it.toDomain()
			if err != nil {
				return domain.MoviePage{}, err
			}
			if filter.Matches(movie) {
				matched = append(matched, movie)
			}
		}
	}

	slices.SortFunc(matched, func(a, b domain.Movie) int {
		if c := strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})

	start := min(filter.Offset(), len(matched))
	end := min(start+filter.PageSize, len(matched))
	return domain.MoviePage{
		Movies:   matched[start:end],
		Total:    int64(len(matched)),
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (domain.Movie, error) {
	out, err := r.client.GetItem(ctx, &awsdynamodb.GetItemInput{
		TableName:      aws.String(r.table),
		Key:            map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return domain.Movie{}, fmt.Errorf("getting movie: %w", err)
	}
	if len(out.Item) == 0 {
		return domain.Movie{}, domain.ErrNotFound
	}
	var item movieItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.Movie{}, fmt.Errorf("decoding movie: %w", err)
	}
	return item.toDomain()
}

func (r *Repository) Create(ctx context.Context, movie domain.Movie) error {
	item, err := attributevalue.MarshalMap(fromDomain(movie))
	if err != nil {
		return fmt.Errorf("encoding movie: %w", err)
	}
	_, err = r.client.PutItem(ctx, &awsdynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	var conditionFailed *types.ConditionalCheckFailedException
	if errors.As(err, &conditionFailed) {
		return domain.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("putting movie: %w", err)
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.client.DeleteItem(ctx, &awsdynamodb.DeleteItemInput{
		TableName:           aws.String(r.table),
		Key:                 map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: id}},
		ConditionExpression: aws.String("attribute_exists(id)"),
	})
	var conditionFailed *types.ConditionalCheckFailedException
	if errors.As(err, &conditionFailed) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("deleting movie: %w", err)
	}
	return nil
}
