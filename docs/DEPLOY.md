# DEPLOY

Authoritative compose workflow by environment.

## Strategy

- `prod`: use GHCR images and deploy `bot` / `downloader` independently.
- `dev`: use local build images and start both services through one compose context.
- This guide intentionally treats independent deployment as a production concern.

## Common prerequisites

### 1. Configure environment

```bash
cp .env.example .env
```

Fill `.env` with Telegram and Cloudflare credentials.

### 2. Configure tags for `prod` in `.env`

Pin to release tags or immutable `sha-*` tags:

```bash
BOT_IMAGE_TAG=sha-<git-sha>
DOWNLOADER_IMAGE_TAG=sha-<git-sha>
```

### 3. GHCR access (when required)

If GHCR images are private, log in before `pull`:

```bash
docker login ghcr.io
```

## prod workflow (independent deployment)

### 1. Initialize downloader session (first deployment on the target host)

```bash
docker compose -f deploy/docker-compose.downloader.yml run --rm --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

### 2. Deploy bot service

```bash
docker compose -f deploy/docker-compose.bot.yml pull
docker compose -f deploy/docker-compose.bot.yml up -d
```

### 3. Deploy downloader service

```bash
docker compose -f deploy/docker-compose.downloader.yml pull
docker compose -f deploy/docker-compose.downloader.yml up -d
```

### 4. Scale downloader (optional)

```bash
docker compose -f deploy/docker-compose.downloader.yml up -d --scale downloader=3
```

## dev workflow (one compose context for both services)

### 1. Set a short compose context

```bash
export COMPOSE_FILE=deploy/docker-compose.bot.yml:deploy/docker-compose.bot.build.yml:deploy/docker-compose.downloader.yml:deploy/docker-compose.downloader.build.yml
```

### 2. Initialize downloader session in the same context (first time)

```bash
docker compose run --rm --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

### 3. Build and start both services

```bash
docker compose build --pull
docker compose up -d
```

### 4. Verify runtime

```bash
docker compose ps
docker compose logs --tail=80 downloader
```

### 5. Scale downloader (optional)

```bash
docker compose up -d --scale downloader=3
```

## Compose wiring checks

Expected service resolution:

```bash
docker compose -f deploy/docker-compose.bot.yml config --services
docker compose -f deploy/docker-compose.downloader.yml config --services
COMPOSE_FILE=deploy/docker-compose.bot.yml:deploy/docker-compose.bot.build.yml:deploy/docker-compose.downloader.yml:deploy/docker-compose.downloader.build.yml docker compose config --services
```

## Operational notes

- Downloader performs startup preflight before queue consumption and requires a ready `tdl` session when login checks are enabled.
- `tdl` session state is persisted in `tgdl-tdl-session` mounted at `/root/.tdl`.
- Compose in this phase uses external Cloudflare Queue + D1 APIs; no local DB container is part of this deployment model.
- Keep `TDL_NAMESPACE` aligned with the namespace used in `tdl login`.
