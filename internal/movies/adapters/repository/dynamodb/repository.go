package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

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

// titleSortIndex is a GSI with a constant partition key (gsi_pk) and
// title_sort (lowercase title + id) as sort key: querying it returns movies
// already in list order, replacing the previous full-table Scan + in-process
// sort. A single-partition GSI caps throughput at one partition's limits —
// fine for this dataset (28k items); at real scale the key would be sharded
// (gsi_pk = MOVIE#0..N) and pages merged.
const (
	titleSortIndex    = "title-sort-index"
	gsiPartitionValue = "MOVIE"
)

type Repository struct {
	client *awsdynamodb.Client
	table  string
}

var _ ports.MovieRepository = (*Repository)(nil)

func NewRepository(client *awsdynamodb.Client, table string) *Repository {
	return &Repository{client: client, table: table}
}

// EnsureTable creates the table (with the title-sort GSI) when absent. On a
// pre-existing table it adds the missing GSI via UpdateTable and backfills
// items written before the index existed, so upgrades need no manual
// migration.
func (r *Repository) EnsureTable(ctx context.Context) error {
	desc, err := r.client.DescribeTable(ctx, &awsdynamodb.DescribeTableInput{TableName: aws.String(r.table)})
	if err == nil {
		return r.ensureIndex(ctx, desc.Table)
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
			{AttributeName: aws.String("gsi_pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("title_sort"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{titleSortIndexDefinition()},
	})
	if err != nil {
		return fmt.Errorf("creating table %q: %w", r.table, err)
	}

	waiter := awsdynamodb.NewTableExistsWaiter(r.client)
	if err := waiter.Wait(ctx, &awsdynamodb.DescribeTableInput{TableName: aws.String(r.table)}, 60*time.Second); err != nil {
		return fmt.Errorf("waiting for table %q: %w", r.table, err)
	}
	return r.waitIndexActive(ctx)
}

func titleSortIndexDefinition() types.GlobalSecondaryIndex {
	return types.GlobalSecondaryIndex{
		IndexName: aws.String(titleSortIndex),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("gsi_pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("title_sort"), KeyType: types.KeyTypeRange},
		},
		Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
	}
}

func (r *Repository) ensureIndex(ctx context.Context, table *types.TableDescription) error {
	for _, gsi := range table.GlobalSecondaryIndexes {
		if aws.ToString(gsi.IndexName) == titleSortIndex {
			if gsi.IndexStatus == types.IndexStatusActive {
				return nil
			}
			return r.waitIndexActive(ctx)
		}
	}

	def := titleSortIndexDefinition()
	_, err := r.client.UpdateTable(ctx, &awsdynamodb.UpdateTableInput{
		TableName: aws.String(r.table),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("gsi_pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("title_sort"), AttributeType: types.ScalarAttributeTypeS},
		},
		GlobalSecondaryIndexUpdates: []types.GlobalSecondaryIndexUpdate{
			{Create: &types.CreateGlobalSecondaryIndexAction{
				IndexName:  def.IndexName,
				KeySchema:  def.KeySchema,
				Projection: def.Projection,
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("adding index %q to table %q: %w", titleSortIndex, r.table, err)
	}
	if err := r.waitIndexActive(ctx); err != nil {
		return err
	}
	return r.backfillIndex(ctx)
}

func (r *Repository) waitIndexActive(ctx context.Context) error {
	const timeout = 5 * time.Minute
	deadline := time.Now().Add(timeout)
	for {
		desc, err := r.client.DescribeTable(ctx, &awsdynamodb.DescribeTableInput{TableName: aws.String(r.table)})
		if err != nil {
			return fmt.Errorf("describing table %q: %w", r.table, err)
		}
		for _, gsi := range desc.Table.GlobalSecondaryIndexes {
			if aws.ToString(gsi.IndexName) == titleSortIndex && gsi.IndexStatus == types.IndexStatusActive {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("index %q on table %q not active after %s", titleSortIndex, r.table, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// backfillIndex rewrites items created before the GSI existed: they lack
// gsi_pk/title_sort and are therefore invisible to the (sparse) index until
// rewritten with the derived attributes.
func (r *Repository) backfillIndex(ctx context.Context) error {
	paginator := awsdynamodb.NewScanPaginator(r.client, &awsdynamodb.ScanInput{
		TableName:        aws.String(r.table),
		FilterExpression: aws.String("attribute_not_exists(gsi_pk)"),
	})
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("scanning items to backfill: %w", err)
		}
		var items []movieItem
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
			return fmt.Errorf("decoding items to backfill: %w", err)
		}
		movies := make([]domain.Movie, 0, len(items))
		for _, it := range items {
			movie, err := it.toDomain()
			if err != nil {
				return err
			}
			movies = append(movies, movie)
		}
		if err := r.CreateMany(ctx, movies); err != nil {
			return fmt.Errorf("backfilling index: %w", err)
		}
	}
	return nil
}

// List queries the title-sort GSI, so items arrive server-sorted and the read
// stops as soon as the page window (offset + page size) is filled — O(window)
// memory and, without filters, O(window) reads instead of O(table). The exact
// total still requires a Select=COUNT pass over the partition (inherent to a
// page+total contract), but that pass transfers no item data. GSI reads are
// eventually consistent, which the async write path already implies.
func (r *Repository) List(ctx context.Context, filter domain.ListFilter) (domain.MoviePage, error) {
	filter = filter.Normalized()
	window := filter.Offset() + filter.PageSize

	filterExpr, names, values := buildListFilter(filter)
	input := &awsdynamodb.QueryInput{
		TableName:                 aws.String(r.table),
		IndexName:                 aws.String(titleSortIndex),
		KeyConditionExpression:    aws.String("gsi_pk = :pk"),
		FilterExpression:          filterExpr,
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	}

	var matched []domain.Movie
	paginator := awsdynamodb.NewQueryPaginator(r.client, input)
	for paginator.HasMorePages() && len(matched) < window {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return domain.MoviePage{}, fmt.Errorf("querying movies: %w", err)
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
			matched = append(matched, movie)
		}
	}
	if len(matched) > window {
		matched = matched[:window]
	}

	total, err := r.count(ctx, filterExpr, names, values)
	if err != nil {
		return domain.MoviePage{}, err
	}

	start := min(filter.Offset(), len(matched))
	return domain.MoviePage{
		Movies:   matched[start:],
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

func (r *Repository) count(ctx context.Context, filterExpr *string, names map[string]string, values map[string]types.AttributeValue) (int64, error) {
	paginator := awsdynamodb.NewQueryPaginator(r.client, &awsdynamodb.QueryInput{
		TableName:                 aws.String(r.table),
		IndexName:                 aws.String(titleSortIndex),
		KeyConditionExpression:    aws.String("gsi_pk = :pk"),
		FilterExpression:          filterExpr,
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
		Select:                    types.SelectCount,
	})
	var total int64
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("counting movies: %w", err)
		}
		total += int64(out.Count)
	}
	return total, nil
}

// buildListFilter translates the domain filter into a server-side
// FilterExpression with the same semantics as domain.ListFilter.Matches:
// case-insensitive title substring (title_lc), case-insensitive genre
// membership (genres_lc string set) and exact year.
func buildListFilter(f domain.ListFilter) (*string, map[string]string, map[string]types.AttributeValue) {
	values := map[string]types.AttributeValue{
		":pk": &types.AttributeValueMemberS{Value: gsiPartitionValue},
	}
	var names map[string]string
	var conds []string
	if f.Title != "" {
		conds = append(conds, "contains(title_lc, :title)")
		values[":title"] = &types.AttributeValueMemberS{Value: strings.ToLower(f.Title)}
	}
	if f.Genre != "" {
		conds = append(conds, "contains(genres_lc, :genre)")
		values[":genre"] = &types.AttributeValueMemberS{Value: strings.ToLower(f.Genre)}
	}
	if f.Year != 0 {
		// "year" is a DynamoDB reserved word and must be aliased.
		conds = append(conds, "#y = :year")
		names = map[string]string{"#y": "year"}
		values[":year"] = &types.AttributeValueMemberN{Value: strconv.Itoa(f.Year)}
	}
	if len(conds) == 0 {
		return nil, nil, values
	}
	return aws.String(strings.Join(conds, " AND ")), names, values
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

// batchWriteSize is the DynamoDB BatchWriteItem hard limit per request.
const batchWriteSize = 25

// batchWriteConcurrency bounds parallel BatchWriteItem calls during bulk
// loads; sequential writes make seeding ~28k movies take several minutes.
const batchWriteConcurrency = 8

const maxBatchRetries = 8

func (r *Repository) CreateMany(ctx context.Context, movies []domain.Movie) error {
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(batchWriteConcurrency)
	for start := 0; start < len(movies); start += batchWriteSize {
		batch := movies[start:min(start+batchWriteSize, len(movies))]
		group.Go(func() error { return r.writeBatch(ctx, batch) })
	}
	return group.Wait()
}

func (r *Repository) writeBatch(ctx context.Context, movies []domain.Movie) error {
	requests := make([]types.WriteRequest, 0, len(movies))
	for _, m := range movies {
		item, err := attributevalue.MarshalMap(fromDomain(m))
		if err != nil {
			return fmt.Errorf("encoding movie: %w", err)
		}
		requests = append(requests, types.WriteRequest{PutRequest: &types.PutRequest{Item: item}})
	}
	pending := map[string][]types.WriteRequest{r.table: requests}
	for attempt := 0; len(pending[r.table]) > 0; attempt++ {
		if attempt > maxBatchRetries {
			return fmt.Errorf("batch writing movies: unprocessed items remain after %d retries", maxBatchRetries)
		}
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
			}
		}
		out, err := r.client.BatchWriteItem(ctx, &awsdynamodb.BatchWriteItemInput{RequestItems: pending})
		if err != nil {
			return fmt.Errorf("batch writing movies: %w", err)
		}
		pending = out.UnprocessedItems
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
