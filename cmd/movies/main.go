package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	moviesv1 "github.com/teuzowebdeveloper9/movie-api/gen/movies/v1"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/grpcserver"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/messaging/rabbitmq"
	dynamorepo "github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/dynamodb"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/memory"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/repository/mongodb"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/adapters/seed"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/config"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/ports"
	"github.com/teuzowebdeveloper9/movie-api/internal/movies/core/service"
	"github.com/teuzowebdeveloper9/movie-api/internal/pkg/logging"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration:", err)
		os.Exit(1)
	}
	logger := logging.New(cfg.LogLevel, "movies")

	if err := run(cfg, logger); err != nil {
		logger.Error("movies service terminated", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo, cleanup, err := buildRepository(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	if cfg.SeedEnabled {
		if err := seed.Run(ctx, repo, cfg.SeedFile, logger); err != nil {
			return fmt.Errorf("seeding: %w", err)
		}
	}

	opts := []service.Option{service.WithLogger(logger)}
	if cfg.EventDriven() {
		publisher, err := rabbitmq.NewPublisher(ctx, cfg.RabbitURL, logger)
		if err != nil {
			return err
		}
		defer func() { _ = publisher.Close() }()
		opts = append(opts, service.WithPublisher(publisher))
		logger.Info("event-driven mode enabled: POST/DELETE are processed asynchronously")
	}
	svc := service.New(repo, opts...)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("listening on port %d: %w", cfg.GRPCPort, err)
	}

	server := grpc.NewServer(grpc.ChainUnaryInterceptor(
		grpcserver.UnaryRecovery(logger),
		grpcserver.UnaryLogging(logger),
	))
	moviesv1.RegisterMoviesServiceServer(server, grpcserver.New(svc))
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	reflection.Register(server)

	group, groupCtx := errgroup.WithContext(ctx)
	if cfg.EventDriven() {
		consumer := rabbitmq.NewConsumer(cfg.RabbitURL, svc, logger)
		group.Go(func() error { return consumer.Run(groupCtx) })
	}
	group.Go(func() error {
		logger.Info("movies grpc server listening", "port", cfg.GRPCPort, "db_driver", cfg.DBDriver)
		return server.Serve(listener)
	})
	group.Go(func() error {
		<-groupCtx.Done()
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		stopped := make(chan struct{})
		go func() {
			server.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			server.Stop()
		}
		return nil
	})

	if err := group.Wait(); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	logger.Info("movies service stopped gracefully")
	return nil
}

func buildRepository(ctx context.Context, cfg config.Config, logger *slog.Logger) (ports.MovieRepository, func(), error) {
	noop := func() {}
	switch cfg.DBDriver {
	case config.DriverMongo:
		client, db, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoDB)
		if err != nil {
			return nil, noop, err
		}
		repo := mongodb.NewRepository(db)
		if err := repo.EnsureIndexes(ctx); err != nil {
			_ = client.Disconnect(ctx)
			return nil, noop, err
		}
		cleanup := func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := client.Disconnect(shutdownCtx); err != nil {
				logger.Error("disconnecting mongodb", "error", err)
			}
		}
		logger.Info("using mongodb repository", "database", cfg.MongoDB)
		return repo, cleanup, nil

	case config.DriverDynamoDB:
		client, err := dynamorepo.NewClient(ctx, dynamorepo.Config{
			Endpoint: cfg.Dynamo.Endpoint,
			Region:   cfg.Dynamo.Region,
			Table:    cfg.Dynamo.Table,
		})
		if err != nil {
			return nil, noop, err
		}
		repo := dynamorepo.NewRepository(client, cfg.Dynamo.Table)
		if err := repo.EnsureTable(ctx); err != nil {
			return nil, noop, err
		}
		logger.Info("using dynamodb repository", "table", cfg.Dynamo.Table, "endpoint", cfg.Dynamo.Endpoint)
		return repo, noop, nil

	default:
		logger.Warn("using in-memory repository: data is lost on restart")
		return memory.New(), noop, nil
	}
}

func healthcheck() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", cfg.GRPCPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	return 0
}
