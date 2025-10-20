# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o olu ./cmd/olu

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
