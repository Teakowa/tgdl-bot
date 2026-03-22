# Agent Instructions

This repository is under active concurrent editing. Do not revert, overwrite, or "clean up" work you did not create.

## Golden Rule

**Always prefix commands with `rtk`**. If RTK has a dedicated filter, it uses it. If not, it passes through unchanged. This means RTK is always safe to use.

**Important**: Even in command chains with `&&`, use `rtk`:
```bash
# ❌ Wrong
git add . && git commit -m "msg" && git push

# ✅ Correct
rtk git add . && rtk git commit -m "msg" && rtk git push
```

## RTK Commands

| Category | Commands |
|----------|----------|
| Tests | vitest, playwright, cargo test |
| Build | next, tsc, lint, prettier |
| Git | status, log, diff, add, commit |
| GitHub | gh pr, gh run, gh issue |
| Package Managers | pnpm, npm, npx |
| Files | ls, read, grep, find |
| Infrastructure | docker, kubectl |
| Network | curl, wget |

## Safety rules

- Preserve existing untracked or modified files unless the user explicitly asks otherwise.
- Prefer additive changes in docs and scaffold files.
- Keep shell usage non-destructive.
- Use ASCII by default.
- Do not introduce shell-based `tdl` invocation in future code; use `exec.CommandContext`.
- Do not add worker-based, web UI, or object storage assumptions to the current deployment model.

## Required implementation order

1. Read the current tree before editing.
2. Update docs and engineering scaffolding first.
3. Add or adjust env examples and deployment samples to match the spec.
4. Add scripts only after the target entrypoints are known.
5. Validate the final tree and confirm no unrelated files were changed.

## Current architecture constraints

- Go is the primary implementation language.
- The system is split into a bot service and a downloader service.
- Cloudflare Queue is split by direction:
  - `CF_QUEUE_ID` for task delivery (`bot -> downloader`)
  - `CF_STATUS_QUEUE_ID` for task status events (`downloader -> bot`)
- Bot and downloader share task state through Cloudflare D1.
- The downloader must perform a session preflight check before pulling queue messages.
- `tdl login` is a manual deployment prerequisite.
- The bot accepts Telegram message URLs plus:
  - `/forward <source_url> <target> [--drop-caption]`
  - `/status <task_id>`
  - `/last`
  - `/queue`
  - `/delete [task_id] [-f|--force]`
  - `/retry <task_id>`
- Polling mode must clear webhook state before `getUpdates`, and polling conflict recovery must keep using `deleteWebhook(drop_pending_updates=false)`.
- Cloudflare Queue is used only as a queue, not as a Workers runtime.
- Current production deployment uses split compose files for bot and downloader.
- Only one active downloader may use the same `TDL_NAMESPACE`/storage because `tdl` session storage is single-process.
- No interactive login flow through the bot.

## Acceptance checklist

- README explains local run for bot and downloader.
- README states the `tdl login` prerequisite.
- README and `docs/DEPLOY.md` describe `prod` split compose (`deploy/docker-compose.bot.yml` and `deploy/docker-compose.downloader.yml`) and `dev` combined compose (`deploy/docker-compose.yml`).
- `docs/ENV.md` documents all spec env vars.
- `docs/API.md` describes the external APIs and internal message/task contracts.
- `.env.example` includes every spec env var with sensible defaults or placeholders.
- Compose docs describe task queue vs status queue direction correctly.
- Compose docs and README state that `prod` does not read the repository `.env`, while `dev` and local scripts do.
- `deploy/docker-compose.bot.yml`, `deploy/docker-compose.downloader.yml`, and `deploy/docker-compose.yml` match the documented service layout.
- `scripts/run-bot.sh` and `scripts/run-downloader.sh` launch the correct Go entrypoints.
- Docs consistently state the single-active-downloader restriction for a shared `TDL_NAMESPACE`/storage.
- No unrelated files are rewritten.

## Tests and validation

- Confirm scripts are executable.
- Confirm compose files reference the expected bot/downloader service layout.
- Confirm the docs and examples mention the `tdl login` prerequisite.
- Confirm queue direction is documented consistently as `bot -> downloader` for tasks and `downloader -> bot` for status.
- Confirm webhook/polling behavior is documented consistently across README and `docs/*`.
- If code is added later, verify startup checks, session preflight before queue consumption, and `exec.CommandContext`-based `tdl` execution.

## Container deployment constraints

- Use separate runtime images for bot and downloader via `deploy/Dockerfile.bot` and `deploy/Dockerfile.downloader`.
- Pin `tdl` to a fixed release version in `deploy/Dockerfile.downloader` (`ARG TDL_VERSION`) and upgrade by rebuilding the downloader image.
- Keep downloader session state on persistent storage (`/root/.tdl` volume).
- Keep container defaults aligned with `.env.example` and current compose files:
  - `TDL_BIN=/usr/local/bin/tdl`
  - `TDL_STORAGE` optional; keep unset unless explicitly required by tdl storage driver config.
- Keep `prod` on GHCR runtime images and `dev` on local build images.
- `prod` compose must remain independent for bot and downloader; `dev` compose remains the combined entrypoint.
- Do not regress to `go run`-based production compose services.
