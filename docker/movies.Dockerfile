FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/movies ./cmd/movies

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/movies /movies
COPY movies.json /data/movies.json
ENV SEED_FILE=/data/movies.json
EXPOSE 50051
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD ["/movies", "healthcheck"]
ENTRYPOINT ["/movies"]
