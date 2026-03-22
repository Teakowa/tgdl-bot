# API

Phase 1 forwarding scope only.

## External APIs

### Telegram Bot API

Used by the bot service to:

- receive updates
- reply to users (with `reply_to_message_id` to quote the original message)
- send task completion or failure notifications

### Cloudflare Queues HTTP API

Used by the bot service to enqueue tasks and by the downloader service to pull tasks.

### `tdl`

Used by the downloader service to perform message forward work.

## Internal data contracts

### Queue message

```json
{
  "task_id": "uuid",
  "chat_id": 123456789,
  "user_id": 123456789,
  "target_chat_id": 123456789,
  "url": "https://t.me/c/xxx/123",
  "created_at": "2026-03-21T00:00:00Z"
}
```

### Task entity

- `task_id`
- `chat_id`
- `user_id`
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
- Bot persists a queued forward task before enqueueing to Cloudflare Queue.
- Downloader performs session preflight before pulling tasks.
- Downloader startup re-enqueues failed/dead-lettered tasks still below retry cap.
- Downloader uses `exec.CommandContext` for `tdl`.
- Downloader captures stdout, stderr, exit code, and timeout behavior.
- Timeout tasks (default 3 hours) are marked `failed` and acked instead of retried.
