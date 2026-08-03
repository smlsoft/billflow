# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o server ./cmd/server \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o lazada_group_key ./cmd/backfill/lazada_group_key \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o lazada_fee_line ./cmd/backfill/lazada_fee_line \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o shopee_multi_order_repair ./cmd/backfill/shopee_multi_order_repair \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o marketplace_order_url_backfill ./cmd/backfill/marketplace_order_url \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o shopee_item_images ./cmd/backfill/shopee_item_images

# Runtime stage
FROM alpine:3.19

# postgresql16-client provides pg_dump used by the in-container backup cron.
# rclone copies approved marketplace source emails using the server-owned
# Google Drive remote; its config is mounted read-only at runtime.
RUN set -eux; \
    for mirror in \
        https://dl-cdn.alpinelinux.org/alpine \
        https://mirror1.hs-esslingen.de/pub/Mirrors/alpine \
        https://mirror.raiolanetworks.com/alpine; do \
        printf '%s/v3.19/main\n%s/v3.19/community\n' "$mirror" "$mirror" > /etc/apk/repositories; \
        if apk add --no-cache ca-certificates tzdata postgresql16-client rclone; then \
            exit 0; \
        fi; \
    done; \
    exit 1

WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/lazada_group_key .
COPY --from=builder /app/lazada_fee_line .
COPY --from=builder /app/shopee_multi_order_repair .
COPY --from=builder /app/marketplace_order_url_backfill .
COPY --from=builder /app/shopee_item_images .

EXPOSE 8090

CMD ["./server"]
