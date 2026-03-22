# DEPLOY

Authoritative split-compose workflow by environment.

This guide uses split compose only (`bot` and `downloader` compose files).

## Compose strategy

- `prod`: use GHCR images from base compose files (no build override).
- `dev`: use local images from build override files (`*.build.yml`).
- Start modes:
  - `combined`: start `bot + downloader` in one command set
  - `single`: start `bot` and `downloader` separately

## Compose file sets

| Environment + mode | Compose files |
| --- | --- |
| `prod + combined` | `deploy/docker-compose.bot.yml` + `deploy/docker-compose.downloader.yml` |
| `prod + single (bot)` | `deploy/docker-compose.bot.yml` |
| `prod + single (downloader)` | `deploy/docker-compose.downloader.yml` |
| `dev + combined` | `deploy/docker-compose.bot.yml` + `deploy/docker-compose.bot.build.yml` + `deploy/docker-compose.downloader.yml` + `deploy/docker-compose.downloader.build.yml` |
| `dev + single (bot)` | `deploy/docker-compose.bot.yml` + `deploy/docker-compose.bot.build.yml` |
| `dev + single (downloader)` | `deploy/docker-compose.downloader.yml` + `deploy/docker-compose.downloader.build.yml` |

## Common prerequisites

### 1. Configure environment

```bash
cp .env.example .env
```

Fill `.env` with Telegram and Cloudflare credentials.

### 2. Configure tags for `prod`

Pin image tags in `.env`:

```bash
BOT_IMAGE_TAG=sha-<git-sha>
DOWNLOADER_IMAGE_TAG=sha-<git-sha>
```

### 3. GHCR access (when required)

If GHCR images are private, log in before `pull`:

```bash
docker login ghcr.io
```

## prod (GHCR images)

### Combined

```bash
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.downloader.yml pull
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.downloader.yml up -d
```

### Single

```bash
docker compose -f deploy/docker-compose.bot.yml pull
docker compose -f deploy/docker-compose.bot.yml up -d

docker compose -f deploy/docker-compose.downloader.yml pull
docker compose -f deploy/docker-compose.downloader.yml up -d
```

### `tdl login` bootstrap (prod downloader context)

```bash
docker compose -f deploy/docker-compose.downloader.yml run --rm --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

### Scale downloader (optional)

```bash
docker compose -f deploy/docker-compose.downloader.yml up -d --scale downloader=3
```

## dev (local build images)

### Combined

```bash
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml build --pull
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml up -d
```

### Single

```bash
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml build --pull
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml up -d

docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml build --pull
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml up -d
```

### `tdl login` bootstrap (dev downloader context)

```bash
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml run --rm --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

### Scale downloader (optional)

```bash
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml up -d --scale downloader=3
```

## Operational notes

- Downloader performs startup preflight before queue consumption and requires a ready `tdl` session when login checks are enabled.
- `tdl` session state is persisted in `tgdl-tdl-session` mounted at `/root/.tdl`.
- Compose in this phase uses external Cloudflare Queue + D1 APIs; no local DB container is part of this deployment model.
- Keep `TDL_NAMESPACE` aligned with the namespace used in `tdl login`.
