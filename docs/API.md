# API

Phase 1 forwarding scope only.

## External APIs

### Telegram Bot API

Used by the bot service to:

- receive updates via webhook or `getUpdates`
- reply to users (with `reply_to_message_id` to quote the original message)
- send task completion or failure notifications
- manage webhook lifecycle via `setWebhook` and `deleteWebhook`

### Cloudflare Queues HTTP API

Used by the bot service to enqueue tasks and by the downloader service to pull tasks.

### Cloudflare D1 API

Used by bot and downloader to read/write task state in a shared database.

### `tdl`

Used by the downloader service to perform message forward work.

## Internal data contracts

### Queue message

```json
{
  "task_id": "uuid",
  "chat_id": 123456789,
  "user_id": 123456789,
  "url": "https://t.me/c/xxx/123",
  "created_at": "2026-03-21T00:00:00Z"
}
```

- `target_chat_id` is optional.
- If `target_chat_id` is omitted (or persisted as `0`), downloader will not pass `--to` and `tdl forward` defaults to `Saved Messages`.

### Task entity

- `task_id`
- `chat_id`
- `user_id`
- `target_chat_id` (optional in JSON output; `0` means unspecified destination)
- `url`
- `status`
- `created_at`
- `updated_at`
- `started_at`
- `finished_at`
- `retry_count`
- `lease_id`
- `output_summary`
- `error_message`
- `exit_code`
- `idempotency_key`

### Status values

- `queued`
- `running`
- `done`
- `failed`
- `retrying`
- `dead_lettered`

## Phase 1 behavior summary

- Bot accepts Telegram URLs only.
- Bot prefers webhook mode when `TELEGRAM_USE_WEBHOOK=true` and `TELEGRAM_WEBHOOK_URL` is set.
- Polling mode always calls `deleteWebhook(drop_pending_updates=false)` before `getUpdates`.
- Polling mode auto-recovers Telegram webhook conflicts (`error_code=409`) by reissuing `deleteWebhook`.
- Webhook requests are accepted only via `POST` and validated with `X-Telegram-Bot-Api-Secret-Token`.
- Bot persists a queued forward task in D1 before enqueueing to Cloudflare Queue.
- Downloader performs session preflight before pulling tasks.
- Downloader atomically claims task ownership (`queued`/`retrying` -> `running`) before execution.
- Downloader startup re-enqueues failed/dead-lettered tasks still below retry cap.
- Downloader uses `exec.CommandContext` for `tdl`.
- Downloader captures stdout, stderr, exit code, and timeout behavior.
- Timeout tasks (default 3 hours) are marked `failed` and acked instead of retried.
