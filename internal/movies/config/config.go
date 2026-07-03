package config

import (
	"fmt"

	"github.com/teuzowebdeveloper9/movie-api/internal/pkg/env"
)

const (
	DriverMongo    = "mongo"
	DriverDynamoDB = "dynamodb"
	DriverMemory   = "memory"
)

type Dynamo struct {
	Endpoint string
	Region   string
	Table    string
}

type Config struct {
	GRPCPort    int
	DBDriver    string
	MongoURI    string
	MongoDB     string
	Dynamo      Dynamo
	RabbitURL   string
	AsyncWrites bool
	SeedEnabled bool
	SeedFile    string
	LogLevel    string
}

func Load() (Config, error) {
	cfg := Config{
		DBDriver: env.String("DB_DRIVER", DriverMongo),
		MongoURI: env.String("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:  env.String("MONGO_DATABASE", "movies"),
		Dynamo: Dynamo{
			Endpoint: env.String("DYNAMODB_ENDPOINT", ""),
			Region:   env.String("AWS_REGION", "us-east-1"),
			Table:    env.String("DYNAMODB_TABLE", "movies"),
		},
		RabbitURL: env.String("RABBITMQ_URL", ""),
		SeedFile:  env.String("SEED_FILE", "movies.json"),
		LogLevel:  env.String("LOG_LEVEL", "info"),
	}

	var err error
	if cfg.GRPCPort, err = env.Int("GRPC_PORT", 50051); err != nil {
		return Config{}, err
	}
	if cfg.AsyncWrites, err = env.Bool("ASYNC_WRITES", true); err != nil {
		return Config{}, err
	}
	if cfg.SeedEnabled, err = env.Bool("SEED_ENABLED", true); err != nil {
		return Config{}, err
	}

	switch cfg.DBDriver {
	case DriverMongo, DriverDynamoDB, DriverMemory:
	default:
		return Config{}, fmt.Errorf("invalid DB_DRIVER=%q (expected %s, %s or %s)",
			cfg.DBDriver, DriverMongo, DriverDynamoDB, DriverMemory)
	}
	return cfg, nil
}

func (c Config) EventDriven() bool {
	return c.AsyncWrites && c.RabbitURL != ""
}
