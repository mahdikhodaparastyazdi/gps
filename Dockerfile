# Dockerfile

# Stage 1: Build the Go application
FROM golang:1.20-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files to the workspace
COPY go.mod go.sum ./

# Download all dependencies. This is cached if the go.mod and go.sum haven't changed
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN go build -o main .

# Stage 2: Run the Go application
FROM alpine:latest

# Copy the binary from the builder stage
COPY --from=builder /app/main /app/main
COPY --from=builder /app/index.html /index.html

# Expose port 8081 to the outside world
EXPOSE 8081

# Run the binary
CMD ["/app/main"]
