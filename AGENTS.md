# Agent Instructions

This repository is under active concurrent editing. Do not revert, overwrite, or "clean up" work you did not create.

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

