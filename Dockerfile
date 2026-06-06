# Stage 1: Build the Go binary
FROM golang:1.26.3-alpine AS builder

# Install system dependencies (git/certs if needed)
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go.mod and go.sum and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o main ./cmd/api

# Stage 2: Create a minimal production image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/main .

# Expose port 8080 (default for Cloud Run)
EXPOSE 8080

# Run the binary
CMD ["./main"]
