# DEPLOY

Authoritative compose workflow by environment.

## Strategy

- `prod`: use GHCR images and deploy `bot` / `downloader` independently.
- `dev`: use local build images and start both services from one compose file.
- This guide intentionally treats independent deployment as a production concern.
- `prod` reads only exported or injected environment variables, not the repository `.env`.
- `dev` compose and local scripts read the repository `.env`.

## Compose entrypoints

- `dev` combined: `deploy/docker-compose.yml`
- `prod` bot: `deploy/docker-compose.bot.yml`
- `prod` downloader: `deploy/docker-compose.downloader.yml`
- compatibility build overlay: `deploy/docker-compose.build.yml` (optional)

## dev prerequisites

### 1. Configure `.env`

```bash
cp .env.example .env
```

Fill `.env` with Telegram and Cloudflare credentials.
Set both queue IDs and keep them different:

- `CF_QUEUE_ID` for task queue (`bot -> downloader`)
- `CF_STATUS_QUEUE_ID` for status queue (`downloader -> bot`)

## prod prerequisites

`prod` compose does not read `../.env`. Inject all required variables from the deployment shell, CI, or secret manager before running compose.

### 1. Export runtime variables

At minimum, export the variables required by the target service:

- bot: `TELEGRAM_BOT_TOKEN`, `CF_ACCOUNT_ID`, `CF_D1_DATABASE_ID`, `CF_QUEUE_ID`, `CF_STATUS_QUEUE_ID`, `CF_API_TOKEN`
- downloader: `CF_ACCOUNT_ID`, `CF_D1_DATABASE_ID`, `CF_QUEUE_ID`, `CF_STATUS_QUEUE_ID`, `CF_API_TOKEN`

Optional variables such as `TELEGRAM_USE_WEBHOOK`, `TELEGRAM_WEBHOOK_URL`, `TELEGRAM_WEBHOOK_SECRET`, `TDL_NAMESPACE`, and queue tuning values can be exported the same way when needed.

Example:

```bash
export TELEGRAM_BOT_TOKEN=...
export CF_ACCOUNT_ID=...
export CF_D1_DATABASE_ID=...
export CF_QUEUE_ID=...
export CF_STATUS_QUEUE_ID=...
export CF_API_TOKEN=...
```

### 2. Export image tags for `prod`

Pin to release tags or immutable `sha-*` tags:

```bash
export BOT_IMAGE_TAG=sha-<git-sha>
export DOWNLOADER_IMAGE_TAG=sha-<git-sha>
```

Or inject the same values from CI secrets/variables instead of exporting them manually.

### 3. GHCR access (when required)

If GHCR images are private, log in before `pull`:

```bash
docker login ghcr.io
```

## prod workflow (independent deployment)

### 1. Validate rendered compose after variable changes

Whenever you change the exported variables or update either prod compose file, render the final config before deployment:

```bash
docker compose -f deploy/docker-compose.bot.yml config
docker compose -f deploy/docker-compose.downloader.yml config
```

After variable or compose changes, rerun `pull` and `up -d` so the containers pick up the new values.

### 2. Initialize downloader session (first deployment on the target host)

```bash
docker compose -f deploy/docker-compose.downloader.yml run --rm --entrypoint /usr/local/bin/tdl downloader login -T qr -n default
```

### 3. Deploy bot service

```bash
docker compose -f deploy/docker-compose.bot.yml pull
docker compose -f deploy/docker-compose.bot.yml up -d
```

### 4. Deploy downloader service

```bash
docker compose -f deploy/docker-compose.downloader.yml pull
docker compose -f deploy/docker-compose.downloader.yml up -d
```

### 5. Downloader replica policy

Run exactly one active downloader replica per `TDL_NAMESPACE`/storage.
Do not use `--scale downloader=...`: `tdl` session storage is single-process and concurrent replicas will fail with database lock errors.
Horizontal scale requires separate `tdl` sessions/storage plus explicit sharding, which is out of scope here.

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

### 4. Downloader replica policy

Run exactly one active downloader replica in dev as well.
Do not use `--scale downloader=...`.

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
- The current deployment supports one active downloader per `TDL_NAMESPACE`/storage only. Atomic task claim prevents duplicate queue ownership, but it does not make `tdl` multi-process safe.
- Compose uses external Cloudflare Queue + D1 APIs; no local DB container is part of this deployment model.
- Downloader does not need Telegram bot token; it publishes task status updates to `CF_STATUS_QUEUE_ID`.
- Bot consumes `CF_STATUS_QUEUE_ID`, refreshes task state from D1, then syncs Telegram task status/reaction.
- Keep `TDL_NAMESPACE` aligned with the namespace used in `tdl login`.
- Queue direction is fixed:
  - `CF_QUEUE_ID`: `bot -> downloader`
  - `CF_STATUS_QUEUE_ID`: `downloader -> bot`
