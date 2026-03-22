# DEPLOY

Authoritative compose workflow by environment.

## Strategy

- `prod`: use GHCR images and deploy `bot` / `downloader` independently.
- `dev`: use local build images and start both services from one compose file.
- This guide intentionally treats independent deployment as a production concern.

## Compose entrypoints

- `dev` combined: `deploy/docker-compose.yml`
- `prod` bot: `deploy/docker-compose.bot.yml`
- `prod` downloader: `deploy/docker-compose.downloader.yml`
- compatibility build overlay: `deploy/docker-compose.build.yml` (optional)

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

## dev workflow (single compose entrypoint for both services)

### 1. Initialize downloader session (first time)

```bash
docker compose -f deploy/docker-compose.yml run --rm --build --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

### 2. Build and start both services

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

### 3. Verify runtime

```bash
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml logs --tail=80 downloader
```

### 4. Scale downloader (optional)

```bash
docker compose -f deploy/docker-compose.yml up -d --scale downloader=3
```

## Compatibility build overlay

For legacy command patterns that still append build overlay:

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.build.yml up -d --build
```

## Compose wiring checks

Expected service resolution:

```bash
docker compose -f deploy/docker-compose.yml config --services
docker compose -f deploy/docker-compose.bot.yml config --services
docker compose -f deploy/docker-compose.downloader.yml config --services
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.build.yml config --services
```

## Operational notes

- Downloader performs startup preflight before queue consumption and requires a ready `tdl` session when login checks are enabled.
- `tdl` session state is persisted in `tgdl-tdl-session` mounted at `/root/.tdl`.
- Compose in this phase uses external Cloudflare Queue + D1 APIs; no local DB container is part of this deployment model.
- Keep `TDL_NAMESPACE` aligned with the namespace used in `tdl login`.
