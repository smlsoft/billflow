# Build stage
FROM golang:1.24-alpine AS builder

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

# The official rclone image is pulled from Docker Hub and supplies the static
# binary without invoking Alpine's package mirrors during this build.
FROM rclone/rclone:1.74.2 AS rclone

# postgres:16-alpine already supplies pg_dump, CA certificates, and timezone
# data required by the backup cron. It is also the image used by this stack's
# postgres service, so customer servers normally already have it locally.
FROM postgres:16-alpine

COPY --from=rclone /usr/local/bin/rclone /usr/local/bin/rclone

# Do not inherit the Postgres entrypoint; this image runs BillFlow's server.
ENTRYPOINT []

WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/lazada_group_key .
COPY --from=builder /app/lazada_fee_line .
COPY --from=builder /app/shopee_multi_order_repair .
COPY --from=builder /app/marketplace_order_url_backfill .
COPY --from=builder /app/shopee_item_images .

EXPOSE 8090

CMD ["./server"]
