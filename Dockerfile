# Imagem única de produção: API Go + frontend compilado (servido pela própria API, mesma origem).
FROM node:22-alpine AS web
RUN corepack enable
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/
RUN pnpm install --frozen-lockfile --filter @chathm/web...
COPY apps/web apps/web
# Só o empacotamento (vite): a checagem de tipos (tsc) já roda no CI e antes de cada publicação, e repeti-la aqui custava ~45 s numa VPS de 1 núcleo.
RUN pnpm --filter @chathm/web exec vite build

FROM golang:1.26-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
# Cache de módulos e de compilação do Go entre construções (BuildKit): mudando só o código do app, só o que mudou recompila (segundos, não minutos).
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# Transcrição das mensagens de voz: whisper.cpp (código aberto, roda na própria máquina, sem custo e sem enviar o áudio a ninguém).
# Compila na máquina que constrói a imagem (-march=native): construa na mesma máquina em que ela vai rodar (é o caso da VPS).
# O modelo "small" quantizado (q5_1, ~180 MB) entende bem o português; para trocar, mude WHISPER_MODEL e WHISPER_MODEL_SHA256.
FROM alpine:3.21 AS whisper
RUN apk add --no-cache build-base cmake git curl
ARG WHISPER_REF=v1.9.5
RUN git clone --depth 1 --branch ${WHISPER_REF} https://github.com/ggml-org/whisper.cpp /whisper
WORKDIR /whisper
RUN cmake -B build -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_SERVER=OFF -DGGML_NATIVE=ON \
 && cmake --build build -j"$(nproc)" --target whisper-cli
ARG WHISPER_MODEL=ggml-small-q5_1.bin
ARG WHISPER_MODEL_SHA256=ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb
RUN curl -fsSL -o /model.bin "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/${WHISPER_MODEL}" \
 && echo "${WHISPER_MODEL_SHA256}  /model.bin" | sha256sum -c -

FROM alpine:3.21
# ffmpeg: conversão de áudio para mensagem de voz e para a transcrição. libstdc++/libgomp: o whisper-cli precisa deles.
RUN apk add --no-cache ffmpeg ca-certificates tzdata libstdc++ libgomp \
 && adduser -D -u 10001 app \
 && mkdir -p /data/attachments && chown -R app /data
COPY --from=api /out/api /app/api
COPY --from=whisper /whisper/build/bin/whisper-cli /usr/local/bin/whisper-cli
COPY --from=whisper /model.bin /opt/whisper/model.bin
COPY --from=web /src/apps/web/dist /app/web
ENV APP_ENV=production HTTP_ADDR=:8080 AUTO_MIGRATE=true WEB_DIR=/app/web STORAGE_DIR=/data/attachments \
    WHISPER_BIN=/usr/local/bin/whisper-cli WHISPER_MODEL=/opt/whisper/model.bin WHISPER_LANG=pt
USER app
VOLUME /data
EXPOSE 8080
# Na partida confere a cada 2 s (start-interval) e vira "healthy" assim que /readyz responde; antes o Docker só olhava depois de 30 s inteiros.
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --start-interval=2s CMD wget -qO- http://127.0.0.1:8080/readyz >/dev/null || exit 1
ENTRYPOINT ["/app/api"]
