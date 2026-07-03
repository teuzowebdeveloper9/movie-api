package config

import (
	"time"

	"github.com/teuzowebdeveloper9/movie-api/internal/pkg/env"
)

const EnvProduction = "production"

type Config struct {
	HTTPPort       int
	MoviesAddr     string
	Env            string
	SwaggerEnabled bool
	RequestTimeout time.Duration
	RateLimitRPM   int
	LogLevel       string
}

func Load() (Config, error) {
	cfg := Config{
		MoviesAddr: env.String("MOVIES_GRPC_ADDR", "localhost:50051"),
		Env:        env.String("APP_ENV", "development"),
		LogLevel:   env.String("LOG_LEVEL", "info"),
	}

	var err error
	if cfg.HTTPPort, err = httpPort(); err != nil {
		return Config{}, err
	}
	if cfg.RequestTimeout, err = env.Duration("REQUEST_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitRPM, err = env.Int("RATE_LIMIT_RPM", 300); err != nil {
		return Config{}, err
	}

	swaggerDefault := cfg.Env != EnvProduction
	if cfg.SwaggerEnabled, err = env.Bool("SWAGGER_ENABLED", swaggerDefault); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func httpPort() (int, error) {
	if port, err := env.Int("HTTP_PORT", 0); err != nil || port != 0 {
		return port, err
	}
	return env.Int("PORT", 8080)
}
