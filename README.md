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

Use split compose by environment:

- `prod` uses GHCR images (no build override).
- `dev` uses local build images (with `*.build.yml` overrides).
- Both environments support:
  - `combined`: start `bot + downloader` in one command
  - `single`: start `bot` and `downloader` separately

| Environment | Combined start | Single start |
| --- | --- | --- |
| `prod` | `docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.downloader.yml up -d` | `docker compose -f deploy/docker-compose.bot.yml up -d` and `docker compose -f deploy/docker-compose.downloader.yml up -d` |
| `dev` | `docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml up -d` | `docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml up -d` and `docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml up -d` |

For full commands (`build`/`pull`/`up`), `tdl login` bootstrap, and scaling examples, see [docs/DEPLOY.md](docs/DEPLOY.md).

## Deployment notes

- Bot and downloader can run on the same machine or separately.
- Task state is stored in D1; both services must point to the same `CF_D1_DATABASE_ID`.
- Downloader must not consume tasks until `tdl` session preflight succeeds.
- Task execution timeout defaults to 3 hours (`TASK_TIMEOUT_MINUTES=180`); timeout tasks are marked failed and removed from queue.
- This phase does not include a web UI, object storage, or worker-based deployment.
- Bot accepts Telegram message URLs only and creates forward tasks.
- In webhook mode, route HTTPS traffic to bot listen address (`TELEGRAM_WEBHOOK_LISTEN_ADDR`, default `:8080`) and configure `TELEGRAM_WEBHOOK_SECRET`.
