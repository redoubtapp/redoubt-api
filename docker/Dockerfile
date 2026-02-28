# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build args for version embedding
ARG VERSION=dev
ARG COMMIT=unknown

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /app/bin/redoubt-api \
    ./cmd/redoubt-api

# Runtime stage - distroless
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/redoubt-api /app/redoubt-api

# Copy migrations (if needed at runtime)
COPY --from=builder /app/internal/db/migrations /app/migrations

EXPOSE 8080 9090 9091

ENTRYPOINT ["/app/redoubt-api"]
