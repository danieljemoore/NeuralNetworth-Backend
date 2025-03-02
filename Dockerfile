# Stage 1: Build the Go application
FROM golang:1.20-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of your application code
COPY . .

# Build the application
RUN go build -o main .

# Stage 2: Create the final lightweight image
FROM alpine:latest

# Install CA certificates for HTTPS
RUN apk update && apk add --no-cache ca-certificates

# Copy the built binary from the builder
COPY --from=builder /app/main /main

# Expose port 8080
EXPOSE 5001

# Command to run the executable
CMD ["/main"]
