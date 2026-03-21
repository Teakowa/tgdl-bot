#!/usr/bin/env sh
set -eu

mode="${APP_MODE:-${1:-bot}}"

mkdir -p /app/data

case "$mode" in
  bot)
    exec /usr/local/bin/tgdl-bot
    ;;
  downloader)
    exec /usr/local/bin/tgdl-downloader
    ;;
  *)
    echo "unknown APP_MODE: $mode (expected: bot|downloader)" >&2
    exit 64
    ;;
esac
