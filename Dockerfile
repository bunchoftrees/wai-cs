FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/ssiq-server ./cmd/server

# ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/ssiq-server /app/ssiq-server
COPY static/ /app/static/
COPY openapi.yaml /app/openapi.yaml

# Create upload temp directory
RUN mkdir -p /tmp/ssiq-uploads

EXPOSE 8080

ENTRYPOINT ["/app/ssiq-server"]
