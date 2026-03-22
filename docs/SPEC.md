# SPEC

Current forwarding scope for TGDL Bot.

## Goals

- Telegram bot entrypoint for URL intake and forward task enqueueing
- Downloader entrypoint for queue consumption and `tdl` forward execution
- D1-backed task persistence shared by bot and downloader
- Dedicated status queue for downloader-to-bot task status synchronization
- Basic idempotency and duplicate protection
- Session preflight before downloader starts consuming tasks
- Startup retry scan that re-enqueues failed or dead-lettered tasks still below retry budget
- Single active downloader execution per `TDL_NAMESPACE`/storage

## Non-goals

- Web frontend
- Cloudflare Workers / Durable Objects runtime dependency
- Object storage upload
- Automatic Telegram re-upload
- Complex permissions or multi-tenant behavior
- Cross-machine queue sharding coordination beyond Cloudflare Queue + D1
- Horizontal scaling of one `tdl` session/storage across multiple active downloader replicas

## Supported bot inputs

- Telegram message URL
- `/forward <source_url> <target> [--drop-caption]`
- `/start`
- `/help`
- `/status <task_id>`
- `/last`
- `/queue`
- `/delete [task_id] [-f|--force]`
- `/retry <task_id>`

## URL and target acceptance rules

- Source URL accepts only Telegram message URLs under `t.me` and `telegram.me`
- Plain-text intake extracts the first valid Telegram message URL from the message body
- `/forward` requires an explicit source URL and explicit target
- `/forward` target accepts:
  - `@username`
  - `username`
  - `https://t.me/<name>`
  - `https://telegram.me/<name>`
  - numeric chat ID such as `-1001234567890`
- Private invite links such as `https://t.me/+...` are rejected

## Task flow

### Bot intake

1. Check allowlist
2. Parse source URL
3. Normalize target peer when `/forward` is used
4. Generate `task_id`
5. Generate `idempotency_key` from `user_id + source_url + target_peer + caption mode`
6. Write task to D1 as `queued`
7. Enqueue to Cloudflare Queue
8. Reply with queue confirmation or error

### Downloader execution

1. Pull a batch from Cloudflare Queue
2. Parse task body
3. Atomically claim task (`queued`/`retrying` -> `running`) with lease ID
4. If claim fails, ack the message because another downloader already owns it
5. Execute `tdl forward`
6. Persist result
7. Publish status event to status queue after each persisted state transition
8. Ack success or retry/fail according to error type
9. On service startup, re-enqueue failed/dead-lettered tasks that still have retry budget

## Forwarding mode notes

- Plain Telegram URL input keeps the default target behavior.
- `/forward` sets `--to <target_peer>` explicitly.
- `/forward` preserves original caption/text by default with `tdl forward --mode direct`.
- `/forward --drop-caption` uses `tdl forward --mode clone --edit '""'`.
- Bot does not expose interactive login.
- Bot prefers webhook mode when configured, and otherwise falls back to long polling.
- Polling mode deletes outgoing webhook before `getUpdates` to satisfy Telegram API mutual exclusion.
- Polling conflict recovery retries after deleting webhook with `drop_pending_updates=false`.
- Queue split is required: `CF_QUEUE_ID` (task queue) and `CF_STATUS_QUEUE_ID` (status queue) must be different values.
- Downloader publishes status events to `CF_STATUS_QUEUE_ID`; bot consumes those events, re-reads task state from D1, and syncs Telegram task status/reaction.

## Deployment prerequisite

The downloader requires a pre-existing `tdl` login session.
Run `tdl login` on each downloader host before starting downloader service on that host.
Only one active downloader process may use the same `TDL_NAMESPACE`/storage because `tdl` session storage is single-process.
