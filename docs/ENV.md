# Environment Variables

All configuration is read from environment variables.

## Telegram

- `TELEGRAM_BOT_TOKEN`: required bot token
- `TELEGRAM_API_BASE`: optional API base URL, defaults to the official Telegram API
- `TELEGRAM_USE_WEBHOOK`: optional boolean, defaults to `false`
- `TELEGRAM_WEBHOOK_URL`: optional webhook URL. Bot enters webhook mode only when this value is set and `TELEGRAM_USE_WEBHOOK=true`; otherwise it falls back to long polling.
- `TELEGRAM_WEBHOOK_SECRET`: required when webhook mode is enabled, checked against `X-Telegram-Bot-Api-Secret-Token`
- `TELEGRAM_WEBHOOK_LISTEN_ADDR`: optional listen address for webhook HTTP server, defaults to `:8080`
- `TELEGRAM_ALLOWED_USER_IDS`: optional comma-separated allowlist of Telegram user IDs

## Cloudflare Queue

- `CF_ACCOUNT_ID`: required Cloudflare account ID
- `CF_QUEUE_ID`: required Cloudflare queue ID
- `CF_API_TOKEN`: required Cloudflare API token
- `CF_QUEUE_BATCH_SIZE`: optional batch size, defaults to `5`
- `CF_QUEUE_VISIBILITY_TIMEOUT_MS`: optional visibility timeout in milliseconds, defaults to `900000`
- `CF_QUEUE_PULL_INTERVAL_MS`: optional pull interval in milliseconds, defaults to `3000`

## Downloader

- `TDL_BIN`: optional path to the `tdl` binary, defaults to `tdl`
- `TDL_NAMESPACE`: optional session namespace, defaults to `default`
- `TDL_STORAGE`: optional `tdl` storage value
- `TDL_LOGIN_REQUIRED`: optional boolean, defaults to `true`
- `TDL_LOGIN_CHECK_ON_START`: optional boolean, defaults to `true`
- `DOWNLOADER_WORKERS`: optional worker count, defaults to `2`
- `TASK_TIMEOUT_MINUTES`: optional per-task timeout, defaults to `180` (3 hours, timeout tasks are marked failed and acked)

## Storage

- `SQLITE_PATH`: optional SQLite database path, defaults to `./data/tasks.db`

## Runtime

- `LOG_LEVEL`: optional log level
- `ENV`: optional runtime environment name

## Phase 1 defaults

The phase 1 scaffold assumes:

- webhook mode only when both `TELEGRAM_USE_WEBHOOK=true` and `TELEGRAM_WEBHOOK_URL` is set; otherwise long polling
- polling mode deletes outgoing webhook (`drop_pending_updates=false`) before reading updates
- polling conflict recovery automatically retries after deleting webhook on Telegram API conflict (`error_code=409`)
- a single local SQLite database
- one `tdl` namespace per downloader deployment
- no interactive login at runtime
- forward target defaults to sender context
