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
- Do not add worker-based, web UI, or object storage assumptions in phase 1.

## Required implementation order

1. Read `tgdl_bot_project_spec.md` and the current tree before editing.
2. Update docs and engineering scaffolding first.
3. Add or adjust env examples and deployment samples to match the spec.
4. Add scripts only after the target entrypoints are known.
5. Validate the final tree and confirm no unrelated files were changed.

## Phase 1 constraints from spec

- Go is the primary implementation language.
- The downloader must perform a session preflight check before pulling queue messages.
- `tdl login` is a manual deployment prerequisite.
- The bot accepts only Telegram message URLs and a small command surface.
- SQLite is the local store.
- Cloudflare Queue is used only as a queue, not as a Workers runtime.
- No interactive login flow through the bot in phase 1.

## Acceptance checklist

- README explains local run for bot and downloader.
- README states the `tdl login` prerequisite.
- `docs/ENV.md` documents all spec env vars.
- `docs/API.md` describes the external APIs and internal message/task contracts.
- `docs/SPEC.md` captures the phase 1 skeleton scope and constraints.
- `.env.example` includes every spec env var with sensible defaults or placeholders.
- `deploy/docker-compose.yml` shows bot, downloader, and persistent volumes for SQLite/downloads.
- `scripts/run-bot.sh` and `scripts/run-downloader.sh` launch the correct Go entrypoints.
- No unrelated files are rewritten.

## Tests and validation

- Confirm scripts are executable.
- Confirm the compose file references the expected service layout.
- Confirm the docs and examples mention the `tdl login` prerequisite.
- If code is added later, verify startup checks and session preflight before queue consumption.

## Container deployment constraints

- Use separate runtime images for bot and downloader via `deploy/Dockerfile.bot` and `deploy/Dockerfile.downloader`.
- Pin `tdl` to a fixed release version in `deploy/Dockerfile.downloader` (`ARG TDL_VERSION`) and upgrade by rebuilding the downloader image.
- Keep downloader session state on persistent storage (`/app/data` and `/root/.tdl` volumes).
- Keep container defaults aligned with `.env.example`:
  - `TDL_BIN=/usr/local/bin/tdl`
  - `DOWNLOAD_DIR=/downloads`
  - `SQLITE_PATH=/app/data/tasks.db`
  - `TDL_STORAGE` optional; keep unset unless explicitly required by tdl storage driver config.
- Do not regress to `go run`-based production compose services.
