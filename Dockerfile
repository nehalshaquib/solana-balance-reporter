FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies for SQLite
RUN apk add --no-cache gcc musl-dev sqlite-dev

# Copy go module files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -o solana-balance-reporter ./cmd

# Create final lightweight image
FROM alpine:latest

# Install runtime dependencies for SQLite
RUN apk --no-cache add ca-certificates tzdata sqlite

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/solana-balance-reporter .

# Copy necessary files
COPY addresses.txt .

# Create directories for volumes
RUN mkdir -p /app/csv /app/logs /app/data

# Set permissions
RUN chmod +x /app/solana-balance-reporter

# Run as non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
RUN chown -R appuser:appgroup /app
USER appuser

# Run the application
CMD ["/app/solana-balance-reporter"] 