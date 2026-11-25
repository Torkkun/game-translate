# shellcheck shell=bash
# scripts/gtl.sh で使う関数。
# game_val <ゲーム> <キー> : games/<ゲーム>/game.local.yaml、無ければ game.yaml から値を読む
game_val() {
  local f v
  for f in "games/$1/game.local.yaml" "games/$1/game.yaml"; do
    [ -f "$f" ] || continue
    v="$(grep -E "^\s*$2:" "$f" | head -1 | sed -E "s/^\s*$2:\s*\"?([^\"]*)\"?\s*$/\1/")"
    if [ -n "$v" ]; then printf '%s\n' "$v"; return; fi
  done
}
