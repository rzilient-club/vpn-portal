FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Download dependencies first (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o vpn-portal .

# ── Runtime image ─────────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache \
    wireguard-tools \
    ca-certificates \
    tzdata

WORKDIR /app

COPY --from=builder /app/vpn-portal .
COPY templates/ templates/
COPY static/    static/
COPY manifest.json .

EXPOSE 8080

CMD ["/app/vpn-portal"]
