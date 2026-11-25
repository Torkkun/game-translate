# セットアップ

gtl は Go のプログラムで、Go や .NET はコンテナ (Docker) の中で動かす。
環境に合わせて、下のどれかを選ぶ。

| 環境 | 翻訳モデル | 手順 |
|---|---|---|
| Windows + WSL2 + NVIDIA GPU | ローカルの PLaMo (無料、1 時間に数千行) | [A](#a-windows--wsl2--nvidia-gpu-おすすめ) |
| GPU なし | Claude API / OpenAI API (有料)、または CPU で PLaMo (遅い) | [B](#b-gpu-が無い場合) |
| macOS / Linux | API か、Linux + NVIDIA GPU なら PLaMo | [C](#c-macos--linux) |
| Windows で WSL を使わない | API か、別の PC の翻訳サーバー | [D](#d-windows-で-wsl-を使わない場合) |

訳文ファイル (`gtl pack` で作った `<言語>.tsv`) が手元にあるゲームは、**翻訳モデルが無くても** Mod を作れる (`gtl unpack` で訳文を合わせて、ゲーム固有の拡張でビルド)。
訳文と Mod の保存先 (データ用リポジトリ) の用意は [RELEASE.md](RELEASE.md)。

## A. Windows + WSL2 + NVIDIA GPU (おすすめ)

1. WSL2 (Ubuntu) と Docker を用意する ([Docker Desktop](https://docs.docker.com/desktop/) の WSL2 バックエンド、または WSL の中に Docker Engine)
2. GPU を Docker から使えるようにする (Docker Desktop なら不要。Docker Engine なら [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/))
3. WSL の中でリポジトリを取得する

   ```bash
   git clone https://github.com/Torkkun/game-translate.git ~/game-translate
   cd ~/game-translate
   ```

4. 翻訳モデル (約 5.3GB) を取得して、翻訳サーバーを起動する

   ```bash
   mkdir -p models
   docker run --rm -u "$(id -u):$(id -g)" -v "$PWD/models:/models" mcr.microsoft.com/devcontainers/go:2-1.27-trixie \
     curl -fL -o /models/plamo-3-translate-8b-Q4_K_M.gguf \
     https://huggingface.co/pfnet/plamo-3-translate-8b-GGUF/resolve/main/plamo-3-translate-8b-Q4_K_M.gguf
   docker compose up -d llm
   ./scripts/gtl.sh ping        # 英文を 1 つ訳せれば OK
   ```

   VRAM は 8GB あれば足りる (Q4_K_M で約 6GB)。モデルのライセンス ([PLaMo Community License](https://huggingface.co/pfnet/plamo-3-translate-8b-GGUF)) を確認してから使う。

5. ゲームの場所が Steam の標準 (`C:\Program Files (x86)\Steam\steamapps\common`) でなければ、
   `games/<game>/game.local.yaml` に WSL から見たパスを書く (git には入らない)

   ```yaml
   install_dir: "/mnt/c/Games/SteamLibrary/steamapps/common/<ゲームのフォルダ>"  # 例: 別の場所に入れた場合
   ```

VS Code の Dev Containers を使う場合は `.devcontainer/` をそのまま開ける (Go・mage・golangci-lint・shellcheck・actionlint・gitleaks・gh・docker CLI 入り)。
翻訳サーバーには 127.0.0.1:8000 でつながる。Mod の組み立てに使う .NET などは入れておらず、組み立てのときにその都度コンテナで動く。

## B. GPU が無い場合

### Claude API / OpenAI API を使う

`gtl.example.yaml` を `gtl.yaml` にコピーして、`llm` を書き換える。API キーは環境変数で渡す (ファイルに書かない)。

```yaml
# gtl.yaml
target_lang: ja
llm:
  provider: anthropic          # OpenAI なら openai
  model: claude-opus-5-5       # OpenAI なら使うモデル名
  api_key_env: ANTHROPIC_API_KEY
  effort: low
  workers: 4                   # 同時に送る数。レート制限に合わせる
```

```bash
export ANTHROPIC_API_KEY=...   # 自分のキー
./scripts/gtl.sh ping
```

- **料金に注意**: 文章量の多いゲーム (数十万〜百万字) は相応の料金がかかる。まず `gtl translate -limit 50` で試し、出力トークン数から見積もる。
  モデルは `llm.model` で変えられる (安いモデルにすると品質が下がることがある)
- 翻訳メモリ (`work/tm.sqlite`) があるので、同じ文は 2 回目から API を呼ばない

### CPU で PLaMo を動かす

```bash
docker compose --profile cpu up -d llm-cpu
./scripts/gtl.sh ping
```

GPU 版の数倍〜十数倍遅い。数百行程度のゲームや、手修正の確認向け。

### Ollama・LM Studio などのローカル LLM

OpenAI 互換の API を持つものなら `provider: openai` で使える。

```yaml
llm:
  provider: openai
  url: http://127.0.0.1:11434   # Ollama
  model: qwen3:14b
```

翻訳専用ではない汎用モデルは、指示 (`internal/llm/prompt.go`) に従わず説明を付けることがある。`gtl warnings` で確かめる。

## C. macOS / Linux

Docker があれば A と同じスクリプトが動く。

- `install_dir` はその OS のパスを書く (Linux の Steam なら `~/.local/share/Steam/steamapps/common/...`)。
  Proton で動かす Windows 版のゲームは、ゲームのフォルダに Mod を入れれば同じように動くことが多い
- macOS では GPU のコンテナが使えないので、翻訳は B (API か CPU)
- プラグインのビルドなど、ゲームの DLL を参照するものは Windows 版のゲームのファイルが必要

## D. Windows で WSL を使わない場合

[Go](https://go.dev/dl/) を Windows に入れれば、gtl は PowerShell からそのまま動く (コンテナを使わない)。

```powershell
git clone https://github.com/Torkkun/game-translate.git
cd game-translate
go build -o gtl.exe ./cmd/gtl
.\gtl.exe ping
```

- `game.local.yaml` の `install_dir` は Windows のパスで書く (`C:/Program Files (x86)/Steam/steamapps/common/...`)
- 翻訳は B の API を使うか、`GTL_LLM_URL` で別の PC の翻訳サーバーを指す
- 開発用のタスクは `go run mage.go <タスク>` で動く (golangci-lint などが手元に無ければ Docker のコンテナで動くので Docker Desktop が要る)。`scripts/*.sh` は bash が要る

## 設定のまとめ

| ファイル | 内容 | git |
|---|---|---|
| `gtl.yaml` | 訳す言語・翻訳モデル・フォント ([gtl.example.yaml](../gtl.example.yaml)) | × |
| `games/<game>/game.yaml` | ゲームごとの既定値 (Steam の標準の場所など。ゲーム固有の拡張と一緒にデータ用リポジトリで管理) | × |
| `games/<game>/game.local.yaml` | 自分の PC でのゲームの場所など | × |
| 環境変数 `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` | API キー | × |
| 環境変数 `GTL_CONFIG` / `GTL_LLM_URL` | 設定ファイルの場所 / 翻訳サーバーの URL | × |
