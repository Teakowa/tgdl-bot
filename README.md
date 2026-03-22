# TGDL Bot

Go-based Telegram message forwarding bot and downloader scaffold for the phase 1 spec.

## What this repo contains

- `cmd/bot`: bot service entrypoint
- `cmd/downloader`: downloader service entrypoint
- `scripts/run-bot.sh`: local launcher for the bot service
- `scripts/run-downloader.sh`: local launcher for the downloader service

## Prerequisites

- Go 1.24+
- `tdl` installed and available on `PATH`
- Telegram bot token
- Cloudflare Queue credentials
- Cloudflare D1 database ID
- Cloudflare API token with Queue + D1 permissions

## Required deployment step

The downloader service depends on an already initialized `tdl` session.
Before running each downloader host for the first time, log in on that host:

```bash
tdl login -T qr -n default
```

Any equivalent `tdl login` flow is acceptable as long as the session namespace used by the downloader is ready.

## Local run

### 1. Configure environment

Copy `.env.example` to `.env` and fill in the required values:

```bash
cp .env.example .env
```

### 2. Run the bot

```bash
./scripts/run-bot.sh
```

The bot service reads Telegram, queue, D1, and runtime settings from the environment.
Runtime mode selection:

- Webhook mode: `TELEGRAM_USE_WEBHOOK=true` and `TELEGRAM_WEBHOOK_URL` is set.
- Polling fallback: all other cases.

Important Telegram constraint: as long as an outgoing webhook exists, `getUpdates` does not receive updates.
To keep polling safe, bot startup always calls `deleteWebhook` with `drop_pending_updates=false`.
If Telegram returns polling conflict (`error_code=409`), bot auto-recovers by deleting webhook and retrying polling.

### 3. Run the downloader

Make sure `tdl login` has been completed first, then start the downloader:

```bash
./scripts/run-downloader.sh
```

The downloader performs a session preflight check before it begins consuming the queue.
The execution model is: `message URL -> queue -> tdl forward`.
On startup, downloader also re-enqueues historical failed/dead-lettered tasks that are still below the retry cap.

## Docker compose

### 1. Backward-compatible one-file deployment

Set `BOT_IMAGE_TAG` and `DOWNLOADER_IMAGE_TAG` in `.env`, then run:

```bash
docker compose -f deploy/docker-compose.yml pull && docker compose -f deploy/docker-compose.yml up -d
```

`deploy/docker-compose.yml` keeps the legacy all-in-one deployment path.

### 2. Split deployment (bot and downloader separate)

Bot only:

```bash
docker compose -f deploy/docker-compose.bot.yml pull && docker compose -f deploy/docker-compose.bot.yml up -d
```

Downloader only:

```bash
docker compose -f deploy/docker-compose.downloader.yml pull && docker compose -f deploy/docker-compose.downloader.yml up -d
```

Scale downloader instances for parallel consumption:

```bash
docker compose -f deploy/docker-compose.downloader.yml up -d --scale downloader=3
```

### 3. Local/dev build overrides

All-in-one local build:

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.build.yml build --pull
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.build.yml up -d
```

Split local build (bot):

```bash
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml build --pull
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml up -d
```

Split local build (downloader):

```bash
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml build --pull
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml up -d --scale downloader=2
```

### 4. Initialize tdl session in downloader container context (first deployment only)

Run this once before expecting downloader consumption to succeed:

```bash
docker compose -f deploy/docker-compose.downloader.yml run --rm --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

This login state is persisted in `tgdl-tdl-session` volume.

## Deployment notes

- Bot and downloader can run on the same machine or separately.
- Task state is stored in D1; both services must point to the same `CF_D1_DATABASE_ID`.
- Downloader must not consume tasks until `tdl` session preflight succeeds.
- Task execution timeout defaults to 3 hours (`TASK_TIMEOUT_MINUTES=180`); timeout tasks are marked failed and removed from queue.
- This phase does not include a web UI, object storage, or worker-based deployment.
- Bot accepts Telegram message URLs only and creates forward tasks.
- In webhook mode, route HTTPS traffic to bot listen address (`TELEGRAM_WEBHOOK_LISTEN_ADDR`, default `:8080`) and configure `TELEGRAM_WEBHOOK_SECRET`.
