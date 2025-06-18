# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o sftpgw .

# Final stage - scratch image
FROM scratch

# Copy the binary from builder
COPY --from=builder /app/sftpgw /sftpgw

# Copy SSL certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Expose SFTP port
EXPOSE 2222

# Set the binary as entrypoint
ENTRYPOINT ["/sftpgw"]