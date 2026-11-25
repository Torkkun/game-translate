#!/usr/bin/env bash
# 開発用のタスク (magefiles/) を、ホストに Go を入れずに動かす。
#   ./scripts/mage.sh                 タスクの一覧
#   ./scripts/mage.sh lint:all        例
# Go のコンテナで mage のタスクを 1 つの実行ファイル (bin/mage) に組み立て、手元で動かす
# (タスクの中から docker・git・gh を使うため、コンテナの中では動かさない)。
# magefiles/ や go.mod が変わったときだけ組み立て直す。Go がある PC では go run mage.go <タスク> でもよい。
set -euo pipefail
cd "$(dirname "$0")/.."
R="$(pwd)"
# Dev Container の中など、mage か Go が手元にあればそのまま使う
if command -v mage >/dev/null 2>&1; then exec mage "$@"; fi
if command -v go >/dev/null 2>&1; then exec go run mage.go "$@"; fi
IMAGE=mcr.microsoft.com/devcontainers/go:2-1.27-trixie
BIN=bin/mage

stale=0
if [ ! -x "$BIN" ]; then
  stale=1
elif [ -n "$(find magefiles mage.go go.mod go.sum internal/devtools -newer "$BIN" -print -quit)" ]; then
  stale=1
fi
if [ "$stale" = 1 ]; then
  mkdir -p bin
  docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp \
    -e GOCACHE="$R/.cache/go-build" -e GOMODCACHE="$R/.cache/go-mod" \
    -v "$R:$R" -w "$R" "$IMAGE" go run mage.go -compile "$R/$BIN" -goos linux -goarch amd64
fi
exec "$BIN" "$@"
