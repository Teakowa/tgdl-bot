FROM golang:1.23-bookworm AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/tgdl-bot ./cmd/bot
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/tgdl-downloader ./cmd/downloader

FROM debian:bookworm-slim AS runtime

ARG TARGETARCH
ARG TDL_VERSION=v0.20.1

ENV APP_MODE=bot
ENV TDL_BIN=/usr/local/bin/tdl
ENV SQLITE_PATH=/app/data/tasks.db

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tar \
    && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
      amd64) asset="tdl_Linux_64bit.tar.gz" ;; \
      arm64) asset="tdl_Linux_arm64.tar.gz" ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/iyear/tdl/releases/download/${TDL_VERSION}/${asset}" -o /tmp/tdl.tar.gz; \
    tar -xzf /tmp/tdl.tar.gz -C /tmp; \
    install -m 0755 /tmp/tdl /usr/local/bin/tdl; \
    rm -rf /tmp/tdl /tmp/tdl.tar.gz

WORKDIR /app

COPY --from=builder /out/tgdl-bot /usr/local/bin/tgdl-bot
COPY --from=builder /out/tgdl-downloader /usr/local/bin/tgdl-downloader
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /usr/local/bin/docker-entrypoint.sh \
    && mkdir -p /app/data

VOLUME ["/app/data"]

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["bot"]
