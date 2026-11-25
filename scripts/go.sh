#!/usr/bin/env bash
# go コマンドを実行する。Go が手元にあれば (Dev Container の中など) そのまま、
# 無ければ Dev Container と同じ Go のイメージで動かす (ホストに Go を入れずに開発するため)。
#   ./scripts/go.sh test ./...
#   ./scripts/go.sh vet ./...
set -euo pipefail
cd "$(dirname "$0")/.."
if command -v go >/dev/null 2>&1; then exec go "$@"; fi
R="$(pwd)"
docker run --rm --network host -u "$(id -u):$(id -g)" -e HOME=/tmp \
  -e GOCACHE="$R/.cache/go-build" -e GOMODCACHE="$R/.cache/go-mod" \
  -v "$R:$R" -w "$R" mcr.microsoft.com/devcontainers/go:2-1.27-trixie go "$@"
