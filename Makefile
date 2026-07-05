.DEFAULT_GOAL := help
COMPOSE ?= docker compose

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: proto
proto: ## Regenerate gRPC code from proto/ (requires protoc + plugins)
	protoc \
		--proto_path=proto \
		--go_out=. --go_opt=module=github.com/teuzowebdeveloper9/movie-api \
		--go-grpc_out=. --go-grpc_opt=module=github.com/teuzowebdeveloper9/movie-api \
		proto/movies/v1/movies.proto

.PHONY: swagger
swagger: ## Regenerate Swagger/OpenAPI docs (requires swag)
	swag init -g cmd/gateway/main.go -o api/openapi --parseInternal

.PHONY: build
build: ## Build both services
	go build -o bin/gateway ./cmd/gateway
	go build -o bin/movies ./cmd/movies

.PHONY: test
test: ## Run unit tests with race detector
	go test -race ./...

.PHONY: cover
cover: ## Run tests and open coverage report
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "coverage report: coverage.html"

.PHONY: test-integration
test-integration: ## Run adapter integration tests via testcontainers (requires Docker)
	go test -race -tags=integration -run Integration -timeout 20m ./internal/movies/adapters/...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: tidy
tidy: ## Tidy go.mod
	go mod tidy

.PHONY: up
up: ## Start the whole stack (gateway + movies + mongodb + rabbitmq)
	$(COMPOSE) up -d --build

.PHONY: up-dynamodb
up-dynamodb: ## Start the stack using LocalStack DynamoDB instead of MongoDB
	$(COMPOSE) -f docker-compose.yml -f docker-compose.localstack.yml up -d --build

.PHONY: tools
tools: ## Start the stack plus mongo-express (DB viewer at :8081)
	$(COMPOSE) --profile tools up -d --build

.PHONY: down
down: ## Stop the stack and remove volumes
	$(COMPOSE) --profile tools down -v --remove-orphans

.PHONY: logs
logs: ## Tail logs from all containers
	$(COMPOSE) logs -f

.PHONY: k8s-apply
k8s-apply: ## Apply Kubernetes manifests
	kubectl apply -k deploy/k8s

.PHONY: k8s-delete
k8s-delete: ## Delete Kubernetes resources
	kubectl delete -k deploy/k8s
