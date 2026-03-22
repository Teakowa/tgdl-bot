# API

Current forwarding behavior and message contracts.

## External APIs

### Telegram Bot API

Used by the bot service to:

- receive updates via webhook or `getUpdates`
- reply to users (with `reply_to_message_id` to quote the original message)
- sync task status messages and source message reactions
- manage webhook lifecycle via `setWebhook` and `deleteWebhook`

### Cloudflare Queues HTTP API

Used by both services with two queues:

- task queue (`CF_QUEUE_ID`): bot enqueues tasks, downloader pulls tasks
- status queue (`CF_STATUS_QUEUE_ID`): downloader publishes task status events, bot pulls status events

### Cloudflare D1 API

Used by bot and downloader to read/write task state in a shared database.

### `tdl`

Used by the downloader service to perform message forward work.
Only one active downloader may use the same `TDL_NAMESPACE`/storage because `tdl` session storage is single-process.

## Internal data contracts

### Task queue message (`CF_QUEUE_ID`)

```json
{
  "task_id": "uuid",
  "chat_id": 123456789,
  "user_id": 123456789,
  "target_peer": "channel_name",
  "url": "https://t.me/c/xxx/123",
  "drop_caption": false,
  "created_at": "2026-03-21T00:00:00Z"
}
```

- `target_peer` is optional.
- If `target_peer` is omitted or empty, downloader will not pass `--to` and `tdl forward` uses the default destination behavior.
- `drop_caption=true` maps to `tdl forward --mode clone --edit '""'`.
- `drop_caption=false` maps to `tdl forward --mode direct`.
- `idempotency_key` is included when the bot enqueues the task.

### Status queue message (`CF_STATUS_QUEUE_ID`)

```json
{
  "task_id": "uuid",
  "status": "running",
  "retry_count": 1,
  "updated_at": "2026-03-21T00:00:00Z"
}
```

- Bot does not trust status queue payload as source of truth.
- Bot always re-reads the task from D1 by `task_id` before updating Telegram messages.

### Task entity

- `task_id`
- `chat_id`
- `user_id`
- `target_peer` (optional; empty means unspecified destination)
- `url`
- `drop_caption`
- `status`
- `created_at`
- `updated_at`
- `started_at`
- `finished_at`
- `source_message_id`
- `status_message_id`
- `retry_count`
- `lease_id`
- `output_summary`
- `error_message`
- `exit_code`
- `idempotency_key`

### Status values

- `queued`
- `running`
- `paused`
- `cancelled`
- `done`
- `failed`
- `retrying`
- `dead_lettered`

Public queue-processing flow currently uses:

- `queued`
- `running`
- `done`
- `failed`
- `retrying`
- `dead_lettered`

## Current behavior summary

- Bot accepts Telegram URLs directly.
- Bot also accepts `/forward <source_url> <target> [--drop-caption]`.
- `/forward` target accepts public Telegram usernames/links and numeric chat IDs.
- Private invite links such as `https://t.me/+...` are rejected.
- Bot prefers webhook mode when `TELEGRAM_USE_WEBHOOK=true` and `TELEGRAM_WEBHOOK_URL` is set.
- Polling mode always calls `deleteWebhook(drop_pending_updates=false)` before `getUpdates`.
- Polling mode auto-recovers Telegram webhook conflicts (`error_code=409`) by reissuing `deleteWebhook`.
- Webhook requests are accepted only via `POST` and validated with `X-Telegram-Bot-Api-Secret-Token`.
- Bot persists a queued forward task in D1 before enqueueing to Cloudflare Queue.
- Bot consumes status queue messages, fetches the latest task state from D1, and updates Telegram status/reaction.
- Downloader performs session preflight before pulling tasks.
- Downloader atomically claims task ownership (`queued`/`retrying` -> `running`) before execution.
- Atomic task claim does not make `tdl` safe for multiple active processes sharing the same session/storage.
- Downloader startup re-enqueues failed/dead-lettered tasks still below retry cap.
- Downloader publishes status events to `CF_STATUS_QUEUE_ID` after each persisted state transition.
- Downloader uses `exec.CommandContext` for `tdl`.
- Downloader captures stdout, stderr, exit code, and timeout behavior.
- Timeout tasks (default 3 hours) are marked `failed` and acked instead of retried.
