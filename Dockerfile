FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go module files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o solana-balance-reporter ./cmd

# Create final lightweight image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/solana-balance-reporter .

# Copy necessary files
COPY addresses.txt .

# Create directories for volumes
RUN mkdir -p /app/csv /app/logs

# Set permissions
RUN chmod +x /app/solana-balance-reporter

# Run as non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
RUN chown -R appuser:appgroup /app
USER appuser

# Run the application
CMD ["/app/solana-balance-reporter"] 