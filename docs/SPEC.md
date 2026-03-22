# SPEC

Phase 1 forwarding scope for TGDL Bot.

## Goals

- Telegram bot entrypoint for URL intake and forward task enqueueing
- Downloader entrypoint for queue consumption and `tdl` forward execution
- D1-backed task persistence shared by bot and downloader
- Basic idempotency and duplicate protection
- Session preflight before downloader starts consuming tasks
- Safe parallel downloader execution via atomic task claim

## Non-goals

- Web frontend
- Cloudflare Workers / Durable Objects runtime dependency
- Object storage upload
- Automatic Telegram re-upload
- Complex permissions or multi-tenant behavior
- Cross-machine queue sharding coordination beyond Cloudflare Queue + D1

## Required services

- Bot service
- Downloader service

## Required startup checks

1. Load configuration
2. Verify `tdl` binary exists
3. Verify `tdl` session namespace is available
4. Start queue consumption only after all checks pass

## Supported bot inputs

- Telegram message URL
- `/start`
- `/help`
- `/status <task_id>`
- `/last`

## URL acceptance rules

- Accept only `t.me` and `telegram.me`
- Extract the first valid Telegram message URL from the message body
- Reject arbitrary command input

## Task flow

### Bot intake

1. Check allowlist
2. Parse URL
3. Generate `task_id`
4. Generate `idempotency_key`
5. Write task to D1 as `queued`
6. Enqueue to Cloudflare Queue
7. Reply with queue confirmation or error

### Downloader execution

1. Pull a batch from Cloudflare Queue
2. Parse task body
3. Atomically claim task (`queued`/`retrying` -> `running`) with lease ID
4. If claim fails, ack the message because another downloader already owns it
5. Execute `tdl` forward
6. Persist result
7. Ack success or retry/fail according to error type
8. On service startup, re-enqueue failed/dead-lettered tasks that still have retry budget

## `tdl` invocation requirements

- Use `exec.CommandContext`
- Do not shell out through a string
- Capture stdout and stderr
- Enforce timeout
- Kill the process on timeout
- Timeout failures are treated as final `failed` tasks and acked out of queue

## Deployment prerequisite

The downloader requires a pre-existing `tdl` login session.
Run `tdl login` on each downloader host before starting downloader service on that host.

## Forwarding mode notes

- Forward target defaults to the sender's Telegram user/chat context.
- Bot does not expose interactive login or arbitrary destination forwarding in this phase.
- Bot prefers webhook mode when configured, and otherwise falls back to long polling.
- Polling mode deletes outgoing webhook before `getUpdates` to satisfy Telegram API mutual exclusion.
