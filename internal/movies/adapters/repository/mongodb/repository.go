package mongodb

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/domain"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
)

const CollectionName = "movies"

func Connect(ctx context.Context, uri, dbName string) (*mongo.Client, *mongo.Database, error) {
	client, err := mongo.Connect(options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second))
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to mongodb: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, nil, fmt.Errorf("pinging mongodb: %w", err)
	}
	return client, client.Database(dbName), nil
}

type Repository struct {
	coll *mongo.Collection
}

var _ ports.MovieRepository = (*Repository)(nil)

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{coll: db.Collection(CollectionName)}
}

func (r *Repository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "title", Value: 1}}},
		{Keys: bson.D{{Key: "genres", Value: 1}}},
		{Keys: bson.D{{Key: "year", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("creating indexes: %w", err)
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter domain.ListFilter) (domain.MoviePage, error) {
	filter = filter.Normalized()
	query := buildQuery(filter)

	total, err := r.coll.CountDocuments(ctx, query)
	if err != nil {
		return domain.MoviePage{}, fmt.Errorf("counting movies: %w", err)
	}

	cursor, err := r.coll.Find(ctx, query, options.Find().
		SetSort(bson.D{{Key: "title", Value: 1}, {Key: "_id", Value: 1}}).
		SetSkip(int64(filter.Offset())).
		SetLimit(int64(filter.PageSize)))
	if err != nil {
		return domain.MoviePage{}, fmt.Errorf("finding movies: %w", err)
	}

	var docs []movieDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return domain.MoviePage{}, fmt.Errorf("decoding movies: %w", err)
	}

	movies := make([]domain.Movie, 0, len(docs))
	for _, d := range docs {
		movies = append(movies, d.toDomain())
	}
	return domain.MoviePage{Movies: movies, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (domain.Movie, error) {
	var doc movieDocument
	err := r.coll.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Movie{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Movie{}, fmt.Errorf("finding movie: %w", err)
	}
	return doc.toDomain(), nil
}

func (r *Repository) Create(ctx context.Context, movie domain.Movie) error {
	_, err := r.coll.InsertOne(ctx, fromDomain(movie))
	if mongo.IsDuplicateKeyError(err) {
		return domain.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("inserting movie: %w", err)
	}
	return nil
}

func (r *Repository) CreateMany(ctx context.Context, movies []domain.Movie) error {
	if len(movies) == 0 {
		return nil
	}
	docs := make([]any, 0, len(movies))
	for _, m := range movies {
		docs = append(docs, fromDomain(m))
	}
	_, err := r.coll.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inserting movies: %w", err)
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	res, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return fmt.Errorf("deleting movie: %w", err)
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func buildQuery(f domain.ListFilter) bson.D {
	q := bson.D{}
	if f.Title != "" {
		q = append(q, bson.E{Key: "title", Value: bson.Regex{Pattern: regexp.QuoteMeta(f.Title), Options: "i"}})
	}
	if f.Genre != "" {
		q = append(q, bson.E{Key: "genres", Value: bson.Regex{Pattern: "^" + regexp.QuoteMeta(f.Genre) + "$", Options: "i"}})
	}
	if f.Year != 0 {
		q = append(q, bson.E{Key: "year", Value: f.Year})
	}
	return q
}
