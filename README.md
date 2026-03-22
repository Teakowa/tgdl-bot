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
- SQLite storage path

## Required deployment step

The downloader service depends on an already initialized `tdl` session.
Before running the downloader for the first time, log in on the target machine:

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

The bot service reads Telegram, queue, storage, and runtime settings from the environment.
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

## Docker build and run

### 1. Build separate images

```bash
docker build -f deploy/Dockerfile.bot -t tgdl-bot:local .
docker build -f deploy/Dockerfile.downloader -t tgdl-downloader:local --build-arg TDL_VERSION=v0.20.1 .
```

`tgdl-bot:local` contains only the bot binary.
`tgdl-downloader:local` contains the downloader binary plus `/usr/local/bin/tdl`.

### 2. Start with docker compose

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

`bot` and `downloader` now run as separate images.

### 3. Initialize tdl session in container context (first deployment only)

Run this once before expecting downloader consumption to succeed:

```bash
docker compose -f deploy/docker-compose.yml run --rm --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

This login state is persisted in Docker volumes (`tgdl-data` and `tgdl-tdl-session`).

## Deployment notes

- The bot and downloader may run on the same machine or separately.
- The downloader must not consume tasks until `tdl` session preflight succeeds.
- Task execution timeout defaults to 3 hours (`TASK_TIMEOUT_MINUTES=180`); timeout tasks are marked failed and removed from queue.
- SQLite is used as the local task store; keep the database on persistent storage.
- This phase does not include a web UI, object storage, or worker-based deployment.
- Bot accepts Telegram message URLs only and creates forward tasks.
- In webhook mode, route HTTPS traffic to bot listen address (`TELEGRAM_WEBHOOK_LISTEN_ADDR`, default `:8080`) and configure `TELEGRAM_WEBHOOK_SECRET`.
