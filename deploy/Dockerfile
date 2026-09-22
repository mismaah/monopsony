# Multi-stage build: front-ends with npm, server with Go, one small runtime image
# that serves the API, the WebSocket and both SPAs.

FROM node:20-alpine AS web
WORKDIR /src
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/
COPY apps/admin/package.json apps/admin/
COPY packages/protocol/package.json packages/protocol/
COPY packages/board-assets/package.json packages/board-assets/
COPY packages/cosmetics/package.json packages/cosmetics/
RUN npm ci
COPY apps apps
COPY packages packages
RUN npm run build -w @monopsony/web && ADMIN_BASE=/admin/ npm run build -w @monopsony/admin

FROM golang:1.25-alpine AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=server /out/server /app/server
COPY --from=web /src/apps/web/dist /app/static
COPY --from=web /src/apps/admin/dist /app/static/admin
# Uploaded cosmetic assets live here (mount a volume or sync to object storage).
RUN mkdir -p /app/data/assets && chown -R app /app/data
USER app
ENV MONOPSONY_ADDR=:8080 MONOPSONY_STATIC_DIR=/app/static MONOPSONY_ENV=prod MONOPSONY_ASSET_DIR=/app/data/assets MONOPSONY_METRICS_ADDR=:9100
EXPOSE 8080 9100
ENTRYPOINT ["/app/server"]
