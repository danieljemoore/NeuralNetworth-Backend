# Stage 1: Build the Go application
ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm as builder
WORKDIR /usr/src/app

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy the source code
COPY . .

# Build the Go application
RUN go build -v -o /run-app .

# Stage 2: Prepare the runtime environment
FROM debian:bookworm

# Install ca-certificates
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy the built application from the builder stage
COPY --from=builder /run-app /usr/local/bin/

# Expose the port your application listens on (If needed)
EXPOSE 5001

# Command to run your application
CMD ["run-app"]
