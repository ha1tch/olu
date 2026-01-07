# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies (gcc needed for SQLite/CGO)
RUN apk add --no-cache git make gcc musl-dev

# Skip checksum verification and allow module updates during build
# This ensures consistent behaviour across Go versions
ENV GONOSUMDB=*
ENV GOPRIVATE=github.com/ha1tch/*
ENV GOFLAGS=-mod=mod

WORKDIR /build

# Copy go mod file (go.sum optional - will be generated)
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled for SQLite support
RUN CGO_ENABLED=1 GOOS=linux go build -a -o olu ./cmd/olu

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/olu .

# Create data and schema directories
RUN mkdir -p data schema

# Expose port
EXPOSE 9090

# Set environment variables
ENV HOST=0.0.0.0
ENV PORT=9090
ENV BASE_DIR=/app/data
ENV SCHEMA_DIR=/app/schema

# Run the application
CMD ["./olu"]
