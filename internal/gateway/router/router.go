package router

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/teuzowebdeveloper9/movie-api/internal/gateway/config"
	"github.com/teuzowebdeveloper9/movie-api/internal/gateway/handler"
)

func New(cfg config.Config, h *handler.MovieHandler, logger *slog.Logger, ready func(ctx context.Context) error) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.ClientIPFromRemoteAddr)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(cfg.RequestTimeout))
	r.Use(httprate.LimitBy(cfg.RateLimitRPM, time.Minute, keyByClientIP))

	r.NotFound(jsonError(http.StatusNotFound, "route_not_found", "route not found"))
	r.MethodNotAllowed(jsonError(http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed"))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, http.StatusOK, "ok")
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if err := ready(req.Context()); err != nil {
			logger.WarnContext(req.Context(), "readiness check failed", "error", err)
			writeStatus(w, http.StatusServiceUnavailable, "movies service unreachable")
			return
		}
		writeStatus(w, http.StatusOK, "ok")
	})

	r.Route("/movies", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Delete("/{id}", h.Delete)
	})

	if cfg.SwaggerEnabled {
		r.Get("/swagger/*", httpSwagger.Handler())
	}

	return r
}

func keyByClientIP(r *http.Request) (string, error) {
	if ip := middleware.GetClientIP(r.Context()); ip != "" {
		return ip, nil
	}
	return "unknown", nil
}

func writeStatus(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(`{"status":"` + message + `"}`))
}

func jsonError(statusCode int, code, message string) http.HandlerFunc {
	body := []byte(`{"error":{"code":"` + code + `","message":"` + message + `"}}`)
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		_, _ = w.Write(body)
	}
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			logger.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
