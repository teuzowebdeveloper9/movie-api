# syntax=docker/dockerfile:1
FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
# Cache mounts persist the module and Go build caches across builds, so a code
# change recompiles only the affected packages instead of the whole module.
# The build cache is shared with the gateway image, so common internal packages
# are compiled once across both services.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/movies ./cmd/movies

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder --link /out/movies /movies
COPY --link movies.json /data/movies.json
ENV SEED_FILE=/data/movies.json
EXPOSE 50051
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD ["/movies", "healthcheck"]
ENTRYPOINT ["/movies"]
