FROM golang:1.27-alpine AS builder

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
FROM alpine:3.24

# Install AWS CLI from the Alpine community repo. A pip install is rejected on
# Alpine 3.24 (Python 3.12) with PEP 668 "externally-managed-environment".
#
# zlib is upgraded by name: the alpine:3.24 tag still ships 1.3.2-r1
# (CVE-2026-4738, fixed in 1.4-r0), and `apk add` keeps an installed package
# that already satisfies what it installs, so a rebuild alone does not pick up
# the fix. Remove the upgrade once the base tag carries zlib 1.4 or newer.
RUN apk add --no-cache curl unzip bash aws-cli && \
    apk upgrade --no-cache zlib

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