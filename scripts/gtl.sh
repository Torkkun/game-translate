#!/usr/bin/env bash
# gtl を組み立てて実行する。
#   ./scripts/gtl.sh <コマンド> [オプション]
# Go が手元にあれば (Dev Container の中など) そのまま。無ければ Dev Container と同じ Go のイメージで動かし
# (WSL にも Windows にも Go を入れずに使うため)、ゲームフォルダ (games/*/game.local.yaml か game.yaml の install_dir) も
# 同じパスでマウントする (install / uninstall 以外では読み取り専用)。
# install / uninstall はゲームフォルダに書き込むので、Dev Container の中 (ゲームフォルダは読み取り専用) でもコンテナで動かす。
# API キー (ANTHROPIC_API_KEY / OPENAI_API_KEY) と GTL_CONFIG / GTL_LLM_URL はコンテナに渡す。
set -euo pipefail
cd "$(dirname "$0")/.."
R="$(pwd)"
case "${1:-}" in
  install | uninstall) ;;
  *)
    if command -v go >/dev/null 2>&1; then
      go build -o bin/gtl ./cmd/gtl
      exec bin/gtl "$@"
    fi
    ;;
esac
IMAGE=mcr.microsoft.com/devcontainers/go:2-1.27-trixie

mounts=()
. scripts/lib.sh
for conf in games/*/game.yaml; do
  d="$(game_val "$(basename "$(dirname "$conf")")" install_dir)"
  [ -d "$d" ] || continue
  mode=ro
  case "${1:-}" in install|uninstall) mode=rw ;; esac
  mounts+=(-v "$d:$d:$mode")
done

docker run --rm --network host -u "$(id -u):$(id -g)" -e HOME=/tmp \
  -e GTL_CONFIG -e GTL_LLM_URL -e ANTHROPIC_API_KEY -e OPENAI_API_KEY \
  -e GOCACHE="$R/.cache/go-build" -e GOMODCACHE="$R/.cache/go-mod" \
  -v "$R:$R" "${mounts[@]}" -w "$R" "$IMAGE" \
  bash -c 'go build -o bin/gtl ./cmd/gtl && exec bin/gtl "$@"' gtl "$@"
