# ================================
# Stage 1: Build static binary
# ================================
FROM golang:1.24-alpine AS builder

# Install build deps
RUN apk add --no-cache \
    git \
    gcc \
    musl-dev \
    make

# Create app user
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy go.mod + go.sum first (cache deps)
COPY go.mod go.sum ./

# Download deps (cached)
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o discord-bot .

# ================================
# Stage 2: Minimal runtime
# ================================
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

# Copy binary
COPY --from=builder /app/discord-bot /discord-bot

# Copy app user (optional, for non-root)
RUN adduser -D appuser
USER appuser
WORKDIR /app

# Run
CMD ["/discord-bot"]
