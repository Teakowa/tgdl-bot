# TGDL Bot

Go-based Telegram download bot and downloader scaffold for the phase 1 spec.

## What this repo contains

- `cmd/bot`: bot service entrypoint
- `cmd/downloader`: downloader service entrypoint
- `scripts/run-bot.sh`: local launcher for the bot service
- `scripts/run-downloader.sh`: local launcher for the downloader service

## Prerequisites

- Go 1.23+
- `tdl` installed and available on `PATH`
- Telegram bot token
- Cloudflare Queue credentials
- A writable download directory
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

### 3. Run the downloader

Make sure `tdl login` has been completed first, then start the downloader:

```bash
./scripts/run-downloader.sh
```

The downloader performs a session preflight check before it begins consuming the queue.

## Deployment notes

- The bot and downloader may run on the same machine or separately.
- The downloader must not consume tasks until `tdl` session preflight succeeds.
- SQLite is used as the local task store; keep the database on persistent storage.
- This phase does not include a web UI, object storage, or worker-based deployment.

