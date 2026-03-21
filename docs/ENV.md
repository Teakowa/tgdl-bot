# Environment Variables

All configuration is read from environment variables.

## Telegram

- `TELEGRAM_BOT_TOKEN`: required bot token
- `TELEGRAM_API_BASE`: optional API base URL, defaults to the official Telegram API
- `TELEGRAM_USE_WEBHOOK`: optional boolean, defaults to `false`
- `TELEGRAM_WEBHOOK_URL`: optional webhook URL when webhook mode is enabled
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
- `TASK_TIMEOUT_MINUTES`: optional per-task timeout, defaults to `60`

## Storage

- `SQLITE_PATH`: optional SQLite database path, defaults to `./data/tasks.db`

## Runtime

- `LOG_LEVEL`: optional log level
- `ENV`: optional runtime environment name

## Phase 2 defaults

The phase 2 scaffold assumes:

- long polling for the bot unless webhook mode is explicitly enabled
- a single local SQLite database
- one `tdl` namespace per downloader deployment
- no interactive login at runtime
- forward target defaults to sender context
