FROM golang:1.23-bookworm AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY *.go ./

# Copy license files to builder stage
COPY LICENSE NOTICE ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o ess-queue-ess .

# Final stage
FROM debian:bookworm-slim

# Install ca-certificates
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/ess-queue-ess .

# Copy license files
COPY --from=builder /build/LICENSE /build/NOTICE ./

# Create directory for queue data
RUN mkdir -p /app/data

# Expose default SQS port
EXPOSE 9324

# Run the application
CMD ["./ess-queue-ess"]
