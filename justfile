# Default recipe
default:
    @just --list

# Run development server with hot reload
dev:
    docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build

# Run development server in detached mode
dev-d:
    docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build

# Run production stack
up:
    docker compose up -d

# Stop all services
down:
    docker compose down

# Stop all services including dev
down-all:
    docker compose -f docker-compose.yml -f docker-compose.dev.yml down

# View logs
logs service="redoubt-api":
    docker compose logs -f {{service}}

# Run database migrations up
migrate-up:
    docker compose exec redoubt-api migrate -path /app/migrations -database "postgres://redoubt:devpassword@postgres:5432/redoubt?sslmode=disable" up

# Run database migrations down (1 step)
migrate-down:
    docker compose exec redoubt-api migrate -path /app/migrations -database "postgres://redoubt:devpassword@postgres:5432/redoubt?sslmode=disable" down 1

# Create a new migration
migrate-create name:
    migrate create -ext sql -dir internal/db/migrations -seq {{name}}

# Generate sqlc code
sqlc:
    sqlc generate

# Run linter
lint:
    golangci-lint run ./...

# Run tests
test:
    go test -v -race -cover ./...

# Run integration tests
test-integration:
    go test -v -race -tags=integration ./...

# Build binary locally
build:
    go build -o bin/redoubt-api ./cmd/redoubt-api

# Generate OpenAPI spec
swagger:
    swag init -g cmd/redoubt-api/main.go -o docs

# View OpenAPI spec with Swagger UI (available at http://localhost:8081)
swagger-ui:
    @echo "Starting Swagger UI at http://localhost:8081"
    docker run --rm -p 8081:8080 -e SWAGGER_JSON=/spec/openapi.yaml -v {{justfile_directory()}}/docs:/spec swaggerapi/swagger-ui

# Clean build artifacts
clean:
    rm -rf bin/ docs/ tmp/

# Format code
fmt:
    go fmt ./...
    goimports -w .

# Tidy dependencies
tidy:
    go mod tidy

# Run all checks (lint, test)
check: lint test

# Build Docker image
docker-build:
    docker build -t redoubt-api:latest -f docker/Dockerfile .

# Shell into API container
shell:
    docker compose exec redoubt-api sh

# Connect to postgres
psql:
    docker compose exec postgres psql -U redoubt -d redoubt

# Connect to redis
redis-cli:
    docker compose exec redis redis-cli
