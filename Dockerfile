# Builder stage
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o gangway ./cmd/gangway/

# Runtime stage
FROM debian:12-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
USER 1001:1001
COPY --from=builder /build/gangway /bin/gangway
ENTRYPOINT ["/bin/gangway"]
