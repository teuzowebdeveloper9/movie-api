// @title			Movie API
// @version		1.0
// @description	REST API for managing movies. The gateway translates REST calls into gRPC requests handled by the Movies microservice (hexagonal architecture, MongoDB/DynamoDB storage, optional event-driven writes through RabbitMQ).
// @contact.name	teuzowebdeveloper9
// @contact.url	https://github.com/teuzowebdeveloper9/movie-api
// @license.name	MIT
// @license.url	https://opensource.org/licenses/MIT
// @BasePath		/
// @tag.name		movies
// @tag.description	Movie management endpoints
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	_ "github.com/teuzowebdeveloper9/movie-api/api/openapi"
	moviesv1 "github.com/teuzowebdeveloper9/movie-api/gen/movies/v1"
	"github.com/teuzowebdeveloper9/movie-api/internal/gateway/config"
	"github.com/teuzowebdeveloper9/movie-api/internal/gateway/handler"
	"github.com/teuzowebdeveloper9/movie-api/internal/gateway/router"
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
	logger := logging.New(cfg.LogLevel, "gateway")

	if err := run(cfg, logger); err != nil {
		logger.Error("gateway terminated", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := grpc.NewClient(cfg.MoviesAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("creating grpc client for %s: %w", cfg.MoviesAddr, err)
	}
	defer func() { _ = conn.Close() }()

	client := moviesv1.NewMoviesServiceClient(conn)
	healthClient := healthpb.NewHealthClient(conn)
	ready := func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		resp, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			return fmt.Errorf("movies service health check: %w", err)
		}
		if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			return fmt.Errorf("movies service reported %s", resp.GetStatus())
		}
		return nil
	}

	h := handler.NewMovieHandler(client, logger)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           router.New(cfg, h, logger, ready),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	logger.Info("gateway listening",
		"port", cfg.HTTPPort,
		"movies_addr", cfg.MoviesAddr,
		"env", cfg.Env,
		"swagger_enabled", cfg.SwaggerEnabled,
	)
	if !cfg.SwaggerEnabled {
		logger.Info("swagger UI disabled: API documentation is not published in production environments")
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down http server: %w", err)
	}
	logger.Info("gateway stopped gracefully")
	return nil
}

func healthcheck() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/healthz", cfg.HTTPPort))
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy: status", resp.StatusCode)
		return 1
	}
	return 0
}
