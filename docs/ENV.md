# Environment Variables

All configuration is read from environment variables.

## Telegram

- `TELEGRAM_BOT_TOKEN`: required for bot service, not required for downloader service
- `TELEGRAM_API_BASE`: optional API base URL, defaults to the official Telegram API
- `TELEGRAM_USE_WEBHOOK`: optional boolean, defaults to `false`
- `TELEGRAM_WEBHOOK_URL`: optional webhook URL. Bot enters webhook mode only when this value is set and `TELEGRAM_USE_WEBHOOK=true`; otherwise it falls back to long polling.
- `TELEGRAM_WEBHOOK_SECRET`: required when webhook mode is enabled, checked against `X-Telegram-Bot-Api-Secret-Token`
- `TELEGRAM_WEBHOOK_LISTEN_ADDR`: optional listen address for webhook HTTP server, defaults to `:8080`
- `TELEGRAM_ALLOWED_USER_IDS`: optional comma-separated allowlist of Telegram user IDs

Bot-only variables:

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_API_BASE`
- `TELEGRAM_USE_WEBHOOK`
- `TELEGRAM_WEBHOOK_URL`
- `TELEGRAM_WEBHOOK_SECRET`
- `TELEGRAM_WEBHOOK_LISTEN_ADDR`
- `TELEGRAM_ALLOWED_USER_IDS`

## Cloudflare (Queue + D1)

- `CF_ACCOUNT_ID`: required Cloudflare account ID
- `CF_D1_DATABASE_ID`: required Cloudflare D1 database ID
- `CF_QUEUE_ID`: required task queue ID (`bot -> downloader`)
- `CF_STATUS_QUEUE_ID`: required status queue ID (`downloader -> bot`), must differ from `CF_QUEUE_ID`
- `CF_API_TOKEN`: required Cloudflare API token (must include Queue + D1 permissions)
- `CF_QUEUE_BATCH_SIZE`: optional batch size, defaults to `5`
- `CF_QUEUE_VISIBILITY_TIMEOUT_MS`: optional visibility timeout in milliseconds, defaults to `900000`
- `CF_QUEUE_PULL_INTERVAL_MS`: optional pull interval in milliseconds, defaults to `3000`

## Downloader

- `TDL_BIN`: optional path to the `tdl` binary, defaults to `tdl`
- `TDL_NAMESPACE`: optional session namespace, defaults to `default`
- `TDL_STORAGE`: optional `tdl` storage value
- `TDL_LOGIN_REQUIRED`: optional boolean, defaults to `true`
- `TDL_LOGIN_CHECK_ON_START`: optional boolean, defaults to `true`
- `DOWNLOADER_WORKERS`: optional worker count, defaults to `1` and must remain `1` because `tdl` session storage cannot be shared by multiple active downloader workers or replicas
- `TASK_TIMEOUT_MINUTES`: optional per-task timeout, defaults to `180` (3 hours, timeout tasks are marked failed and acked)

Downloader-only variables:

- `TDL_BIN`
- `TDL_NAMESPACE`
- `TDL_STORAGE`
- `TDL_LOGIN_REQUIRED`
- `TDL_LOGIN_CHECK_ON_START`
- `DOWNLOADER_WORKERS`
- `TASK_TIMEOUT_MINUTES`

## Runtime

- `LOG_LEVEL`: optional log level
- `ENV`: optional runtime environment name

## Deployment (docker compose)

- `BOT_IMAGE_TAG`: optional bot image tag for `ghcr.io/teakowa/tgdl-bot`, defaults to `latest`
- `DOWNLOADER_IMAGE_TAG`: optional downloader image tag for `ghcr.io/teakowa/tgdl-downloader`, defaults to `latest`
- `prod` compose (`deploy/docker-compose.bot.yml` and `deploy/docker-compose.downloader.yml`) reads these values and all runtime configuration from the deployment environment or CI-injected variables, not from the repository `.env`
- `dev` compose (`deploy/docker-compose.yml`) continues to read `../.env`
- local scripts (`scripts/run-bot.sh` and `scripts/run-downloader.sh`) also load `.env` when present

## Current defaults and constraints

The current deployment assumes:

- webhook mode only when both `TELEGRAM_USE_WEBHOOK=true` and `TELEGRAM_WEBHOOK_URL` is set; otherwise long polling
- polling mode deletes outgoing webhook (`drop_pending_updates=false`) before reading updates
- polling conflict recovery automatically retries after deleting webhook on Telegram API conflict (`error_code=409`)
- D1 is the single task store for bot and downloader
- `CF_QUEUE_ID` and `CF_STATUS_QUEUE_ID` are split by direction and cannot be shared
- downloader publishes status events to `CF_STATUS_QUEUE_ID`; bot consumes and syncs Telegram status from D1
- one active downloader per `tdl` namespace/storage
- atomic task claim prevents duplicate queue ownership, not multi-process `tdl` access
- no interactive login at runtime
- forward target defaults to sender context
