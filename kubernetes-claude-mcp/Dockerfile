FROM golang:1.20-alpine AS builder

# Install git and CA certificates for private repos if needed
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mcp-server ./cmd/server

# Use a minimal alpine image for the final stage
FROM alpine:3.23

# Install AWS CLI v1 (works on Alpine)
RUN apk add --no-cache curl unzip bash python3 py3-pip && \
    pip install awscli

# Create non-root user
RUN addgroup -g 1001 mcp && \
    adduser -D -u 1001 -G mcp mcp

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/mcp-server .

# Use non-root user
USER mcp

# Expose app port
EXPOSE 8080

# Run the MCP server
CMD ["./mcp-server", "--config", "/app/config.yaml"]