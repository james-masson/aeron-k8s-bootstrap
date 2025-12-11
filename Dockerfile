FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy Go module file
COPY go.mod ./

# Copy source code
COPY aeron-k8s-bootstrap.go ./

# Download dependencies and build the binary
RUN --mount=type=cache,target=/root/.cache go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o aeron-k8s-bootstrap .

# Final stage - minimal runtime image
FROM alpine:latest

# Install ca-certificates for HTTPS connections to Kubernetes API
RUN apk --no-cache add ca-certificates

WORKDIR /

# Copy the binary from builder stage
COPY --from=builder /app/aeron-k8s-bootstrap /usr/local/bin/aeron-k8s-bootstrap

# Create the aeron directory
RUN mkdir -p /etc/aeron

# Run as non-root user for security
RUN adduser -D -s /bin/sh aeron
USER aeron

ENTRYPOINT ["/usr/local/bin/aeron-k8s-bootstrap"]
