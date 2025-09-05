FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy Go module file
COPY go.mod ./

# Copy source code
COPY aeron-md-bootstrap.go ./

# Download dependencies and build the binary
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o aeron-md-bootstrap .

# Final stage - minimal runtime image
FROM alpine:latest

# Install ca-certificates for HTTPS connections to Kubernetes API
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/aeron-md-bootstrap .

# Create the aeron directory
RUN mkdir -p /etc/aeron

# Run as non-root user for security
RUN adduser -D -s /bin/sh aeron
USER aeron

ENTRYPOINT ["./aeron-md-bootstrap"]