# DEPLOY

Authoritative compose workflow for local development/testing and production rollout.

## Compose file matrix

| Scenario | Compose files | Services | Recommendation |
| --- | --- | --- | --- |
| Split production deployment | `deploy/docker-compose.bot.yml` and `deploy/docker-compose.downloader.yml` | `bot` and `downloader` deployed independently | Primary path |
| Split local/dev build | `deploy/docker-compose.bot.yml` + `deploy/docker-compose.bot.build.yml`, and `deploy/docker-compose.downloader.yml` + `deploy/docker-compose.downloader.build.yml` | `bot` and `downloader` with local images | Primary local test path |
| Single-host compatibility mode | `deploy/docker-compose.yml` | `bot` + `downloader` in one project | Compatibility path |
| Single-host local/dev build | `deploy/docker-compose.yml` + `deploy/docker-compose.build.yml` | `bot` + `downloader` with local images | Compatibility local test path |

## Local dev/test flow (compose + local build)

### 1. Configure environment

```bash
cp .env.example .env
```

Fill `.env` with Telegram and Cloudflare credentials.

### 2. Build local images (split path)

```bash
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml build --pull
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml build --pull
```

### 3. Bootstrap `tdl` login in downloader container context (first time per host/project)

Use the same compose file set as the downloader local-build path:

```bash
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml run --rm --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

### 4. Start services

```bash
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml up -d
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml up -d
```

### 5. Verify compose wiring and runtime

```bash
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml config --services
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml config --services
docker compose -f deploy/docker-compose.bot.yml -f deploy/docker-compose.bot.build.yml ps
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml ps
docker compose -f deploy/docker-compose.downloader.yml -f deploy/docker-compose.downloader.build.yml logs --tail=80 downloader
```

## Production deployment flow (recommended split mode)

### 1. Pin image tags in `.env`

Set these to a release tag or immutable `sha-*` tag published to GHCR:

```bash
BOT_IMAGE_TAG=sha-<git-sha>
DOWNLOADER_IMAGE_TAG=sha-<git-sha>
```

### 2. Ensure downloader session is initialized before consumption

Run once on each downloader deployment context:

```bash
docker compose -f deploy/docker-compose.downloader.yml run --rm --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

### 3. Deploy bot and downloader separately

```bash
docker compose -f deploy/docker-compose.bot.yml pull
docker compose -f deploy/docker-compose.bot.yml up -d

docker compose -f deploy/docker-compose.downloader.yml pull
docker compose -f deploy/docker-compose.downloader.yml up -d
```

### 4. Scale downloader workers (optional)

```bash
docker compose -f deploy/docker-compose.downloader.yml up -d --scale downloader=3
```

## Compatibility mode: single-host one-file compose

Use this path only when you intentionally want both services in one compose project.

```bash
docker compose -f deploy/docker-compose.yml pull
docker compose -f deploy/docker-compose.yml up -d
```

Local build variant:

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.build.yml build --pull
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.build.yml up -d
```

## Operational notes

- Downloader executes startup preflight before queue consumption and requires a ready `tdl` session when login checks are enabled.
- `tdl` session state is persisted in the named volume `tgdl-tdl-session` mounted at `/root/.tdl`.
- Current compose setup uses external Cloudflare Queue + D1 APIs; no local DB container is part of phase 1 compose.
- Keep `TDL_NAMESPACE` consistent with the namespace used during `tdl login`.
