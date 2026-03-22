# TGDL Bot

Go-based Telegram message forwarding bot and downloader for the current production deployment model.

## What this repo contains

- `cmd/bot`: bot service entrypoint
- `cmd/downloader`: downloader service entrypoint
- `scripts/run-bot.sh`: local launcher for the bot service
- `scripts/run-downloader.sh`: local launcher for the downloader service

## Prerequisites

- Go 1.24+
- `tdl` installed and available on `PATH`
- Telegram bot token (bot service only)
- Cloudflare Queue credentials (task queue + status queue)
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
Downloader does not call Telegram Bot API directly.
On startup, downloader also re-enqueues historical failed/dead-lettered tasks that are still below the retry cap.
Only one active downloader may use the same `TDL_NAMESPACE`/storage because `tdl` session storage is single-process.

## Docker compose

Use split compose by environment:

- `prod` uses GHCR images and is intended for independent deployment (`bot` and `downloader` separately).
- `dev` uses local build images and is intended for one compose file that starts both services.

`prod` does not read the repository `.env` file. Export the required variables in the deployment shell or inject them from CI/secrets management before running compose. `dev` continues to use `.env`.

`prod` start:

```bash
export BOT_IMAGE_TAG=sha-<git-sha>
export DOWNLOADER_IMAGE_TAG=sha-<git-sha>
export TELEGRAM_BOT_TOKEN=...
export CF_ACCOUNT_ID=...
export CF_D1_DATABASE_ID=...
export CF_QUEUE_ID=...
export CF_STATUS_QUEUE_ID=...
export CF_API_TOKEN=...

docker compose -f deploy/docker-compose.bot.yml pull
docker compose -f deploy/docker-compose.bot.yml up -d

docker compose -f deploy/docker-compose.downloader.yml pull
docker compose -f deploy/docker-compose.downloader.yml up -d
```

`dev` start (single compose entrypoint):

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

Compatibility build overlay (optional for legacy command patterns):

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.build.yml up -d --build
```

For full commands (`build`/`pull`/`up`) and `tdl login` bootstrap, see [docs/DEPLOY.md](docs/DEPLOY.md).

## Deployment notes

- Bot and downloader can run on the same machine or separately.
- Task state is stored in D1; both services must point to the same `CF_D1_DATABASE_ID`.
- `CF_QUEUE_ID` is the task queue (`bot -> downloader`).
- `CF_STATUS_QUEUE_ID` is the status queue (`downloader -> bot`) and must be different from `CF_QUEUE_ID`.
- Downloader must not consume tasks until `tdl` session preflight succeeds.
- Downloader writes task state to D1 and emits status events to `CF_STATUS_QUEUE_ID`.
- Bot consumes `CF_STATUS_QUEUE_ID`, refreshes task state from D1, and updates Telegram status/reaction.
- Atomic task claim only prevents duplicate queue ownership; it does not make `tdl` safe for multi-process access to the same session/storage.
- Only one active downloader replica may use the same `TDL_NAMESPACE`/storage. Horizontal scale requires separate `tdl` sessions/storage plus explicit sharding, which is out of scope here.
- Task execution timeout defaults to 3 hours (`TASK_TIMEOUT_MINUTES=180`); timeout tasks are marked failed and removed from queue.
- This system does not include a web UI, object storage, or worker-based deployment.
- Bot accepts Telegram message URLs only and creates forward tasks.
- In webhook mode, route HTTPS traffic to bot listen address (`TELEGRAM_WEBHOOK_LISTEN_ADDR`, default `:8080`) and configure `TELEGRAM_WEBHOOK_SECRET`.
