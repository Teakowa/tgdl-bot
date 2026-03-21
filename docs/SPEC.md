# SPEC

Phase 1 skeleton scope for TGDL Bot.

## Goals

- Telegram bot entrypoint for URL intake and task enqueueing
- Downloader entrypoint for queue consumption and `tdl` execution
- SQLite-backed task persistence
- Basic idempotency and duplicate protection
- Session preflight before downloader starts consuming tasks

## Non-goals

- Web frontend
- Cloudflare Workers / Durable Objects / D1 dependency
- Object storage upload
- Automatic Telegram re-upload
- Complex permissions or multi-tenant behavior
- Cross-machine sharding

## Required services

- Bot service
- Downloader service

## Required startup checks

1. Load configuration
2. Verify `tdl` binary exists
3. Verify download directory is writable
4. Verify `tdl` session namespace is available
5. Start queue consumption only after all checks pass

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
5. Write task to SQLite as `queued`
6. Enqueue to Cloudflare Queue
7. Reply with queue confirmation or error

### Downloader execution

1. Pull a batch from Cloudflare Queue
2. Parse task body
3. Check task state
4. Skip and ack already completed tasks
5. Mark task as `running`
6. Execute `tdl`
7. Persist result
8. Ack success or retry/fail according to error type

## `tdl` invocation requirements

- Use `exec.CommandContext`
- Do not shell out through a string
- Capture stdout and stderr
- Enforce timeout
- Kill the process on timeout

## Deployment prerequisite

The downloader requires a pre-existing `tdl` login session.
Run `tdl login` on the target machine before starting the downloader service.

