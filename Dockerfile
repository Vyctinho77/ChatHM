# Imagem única de produção: API Go + frontend compilado (servido pela própria API, mesma origem).
FROM node:22-alpine AS web
RUN corepack enable
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/
RUN pnpm install --frozen-lockfile --filter @chathm/web...
COPY apps/web apps/web
RUN pnpm --filter @chathm/web build

FROM golang:1.26-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.21
# ffmpeg: conversão de áudio para mensagem de voz.
RUN apk add --no-cache ffmpeg ca-certificates tzdata \
 && adduser -D -u 10001 app \
 && mkdir -p /data/attachments && chown -R app /data
COPY --from=api /out/api /app/api
COPY --from=web /src/apps/web/dist /app/web
ENV APP_ENV=production HTTP_ADDR=:8080 AUTO_MIGRATE=true WEB_DIR=/app/web STORAGE_DIR=/data/attachments
USER app
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/readyz >/dev/null || exit 1
ENTRYPOINT ["/app/api"]
