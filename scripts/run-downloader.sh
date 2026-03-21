#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

export GO111MODULE="${GO111MODULE:-on}"

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

exec go run ./cmd/downloader

