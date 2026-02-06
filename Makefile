.PHONY: build run test clean docker-up docker-down dev-token lint

# Build the Go binary
build:
	go build -o bin/ssiq-server ./cmd/server

# Run locally (requires Postgres running)
run: build
	./bin/ssiq-server

# Run all tests
test:
	go test -v -race -count=1 ./...

# Run tests with coverage
test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

# Start all services with Docker Compose
docker-up:
	docker compose up --build -d

# Stop all services
docker-down:
	docker compose down

# Stop all services and remove volumes
docker-clean:
	docker compose down -v

# View logs
logs:
	docker compose logs -f api

# Generate a dev JWT token for the demo tenant
dev-token:
	@curl -s -X POST http://localhost:8080/dev/token \
		-H "Content-Type: application/json" \
		-d '{"tenant_id": "550e8400-e29b-41d4-a716-446655440000", "user_id": "00000000-0000-0000-0000-000000000001", "role": "admin"}' | jq .

# Upload the synthetic CSV
dev-upload:
	@TOKEN=$$(curl -s -X POST http://localhost:8080/dev/token \
		-H "Content-Type: application/json" \
		-d '{"tenant_id": "550e8400-e29b-41d4-a716-446655440000", "user_id": "00000000-0000-0000-0000-000000000001", "role": "admin"}' | jq -r '.token') && \
	curl -s -X POST http://localhost:8080/api/v1/uploads \
		-H "Authorization: Bearer $$TOKEN" \
		-F "file=@testdata/logistics_site_data.csv" | jq .

# Run Go formatting
fmt:
	gofmt -w .

# Run Go vet
vet:
	go vet ./...

# Download dependencies
deps:
	go mod tidy
	go mod download
