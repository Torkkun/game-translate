# game-translate

PC ゲーム (主に Steam) を、LLM の機械翻訳と手修正で別の言語にするための共通ツール `gtl`。
日本語に対応していないゲームを日本語で遊ぶことを主な目的にしているため、既定は英語 → 日本語。訳す言語・翻訳モデル・フォントは設定で切り替えられる。

このリポジトリに入っているのは、どのゲームにも共通する部分だけ。

- 翻訳モデルの呼び出し (ローカルの [PLaMo 3 Translate](https://huggingface.co/pfnet/plamo-3-translate-8b-GGUF) / OpenAI 互換 API / Claude API)
- 翻訳の流れ (翻訳メモリ・用語集・キャラクター表による口調の指示と確認・書式記号の検証・再試行・手修正の取り込み)
- Mod の導入と取り外し (上書きするファイルの退避と記録)
- コンテナで動かす開発環境と、Claude Code などのコーディングエージェントで作業するための決まり ([CLAUDE.md](CLAUDE.md))

ゲームごとの抽出・ビルドのコード (ゲームのデータ形式の読み書き、プラグイン、フックなど) は **ゲーム固有の拡張** として別に持ち、
このリポジトリには含めない ([下の説明](#ゲーム固有の拡張))。

> **注意**: 非公式のツールで、各ゲームの開発元・販売元とは関係がない。使うには対象のゲームを正規に所有している必要がある。
> 作った Mod はゲームのデータを含むことがあるので、個人で使う範囲にとどめ、配布しないこと ([docs/LICENSES.md](docs/LICENSES.md))。

| 文書 | 内容 |
|---|---|
| [docs/SETUP.md](docs/SETUP.md) | セットアップ (GPU なし・WSL なしを含む) |
| [docs/ENGINES.md](docs/ENGINES.md) | エンジン別の多言語化の方法 (ゲーム固有の拡張を作るときの出発点) |
| [docs/RESEARCH.md](docs/RESEARCH.md) | 新しいゲームの調査手順 (既知の方法が当てはまらないとき) |
| [docs/CHARACTERS.md](docs/CHARACTERS.md) | キャラクター表と口調の確認 (話者ごとの一人称・語尾のずれを防ぐ) |
| [docs/LICENSES.md](docs/LICENSES.md) | ライセンスと配布の注意 |
| [docs/RELEASE.md](docs/RELEASE.md) | CI、ツールのリリース、訳文と Mod の保存 |
| [CLAUDE.md](CLAUDE.md) | コーディングエージェントで作業するときの決まり |

**このリポジトリにゲームのデータ・訳文・ゲーム固有のコードは入っていない。** 原文やゲームのファイルは、自分が持っているゲームから取り出して使う。
作った訳文・用語集・手修正・ゲーム固有の拡張・Mod 本体は、使う人が非公開のデータ用リポジトリに保存する仕組みを用意している
(`mage data:save`、[docs/RELEASE.md](docs/RELEASE.md))。

## 対応実績

このツールとゲーム固有の拡張で日本語化を試したゲームと、その方式。拡張・訳文・Mod 本体はこのリポジトリには含めない。
エンジンごとの方法は [docs/ENGINES.md](docs/ENGINES.md)。

| ゲーム | エンジン | 方式 | 訳 |
|---|---|---|---|
| Blacksad: Under the Skin | Unity (Mono) | BepInEx プラグインで台詞・メニューを差し替え | 機械翻訳 + 手修正 |
| L.A. Noire | 独自エンジン | PC 版のデータに同梱されている公式日本語を英語の枠へ | 公式訳 |
| Grim Fandango Remastered | 独自エンジン | 描画フック DLL で日本語を描き、テキストを置き換え | 機械翻訳 + 手修正 |
| Kathy Rain | Adventure Game Studio | 新しい AGS エンジン + UTF-8 の翻訳ファイル | 機械翻訳 + 手修正 |
| The Roottrees are Dead | Godot 4 | 言語の枠を差し替え + 日本語検索の追加スクリプト | 機械翻訳 |
| Subverse | Unreal Engine 4 | 訳文とフォントを入れた追加 pak | 機械翻訳 |
| Hotline Miami 2: Wrong Number | 独自エンジン | 同梱の公式日本語を英語の枠へ + フォント | 公式訳 |
| Oxygen Not Included | Unity (Mono) | 公式の翻訳 Mod の仕組み | 既存のファン翻訳 (MIT) + 追加訳 |

## 基本の考え方

- Go・.NET などはコンテナの中だけで動く。コンテナは役割で分けている
  | コンテナ | 使うとき |
  |---|---|
  | 翻訳サーバー (`compose.yaml` の `llm`) | いつも (GPU で PLaMo を動かす) |
  | Dev Container (`.devcontainer/`) | 普段の開発 (Go・mage・golangci-lint・shellcheck・actionlint・gitleaks・gh・docker CLI) |
  | その都度起動するコンテナ | Mod の組み立てのときだけ (.NET SDK・MinGW・Python など、ゲーム固有の拡張が使うもの) |

  Dev Container を使わない場合も、`./scripts/gtl.sh`・`go.sh`・`mage.sh` が Go のコンテナで動かすので、ホストに Go は要らない
- ゲームフォルダは読み取り専用で扱う。書き込むのは `gtl install` / `gtl uninstall` のときだけで、
  上書きするファイルは `work/<game>/backup/` に退避し、入れたものを `gtl-install.json` に記録する
- Mod は `dist/<game>` に作ってから入れる

## 使い方の流れ

```bash
# 0. 設定 (無ければ英語 → 日本語、ローカルの PLaMo)
cp gtl.example.yaml gtl.yaml        # 言語・モデル・フォントを書き換える
./scripts/gtl.sh ping               # 翻訳モデルの確認

# 1. ゲームから原文を取り出す (<game> はゲーム名。ゲーム固有の拡張のコマンド。入っているものは gtl help に出る)
./scripts/gtl.sh <抽出コマンド> ...            # → work/<game>/source.jsonl

# 2a. 訳文ファイル (gtl pack で作った TSV) が手元にあれば、それを合わせるだけ (翻訳モデル不要)
./scripts/gtl.sh unpack -src work/<game>/source.jsonl -tsv games/<game>/ja.tsv -out games/<game>/ja.jsonl

# 2b. 無いゲーム・別の言語は翻訳する (中断しても再実行で続きから)
./scripts/gtl.sh translate -in work/<game>/source.jsonl -out games/<game>/ko.jsonl \
  -target ko -glossary games/<game>/glossary.yaml

# 3. 警告を確認して手で直す (fixes.tsv に「ID<TAB>訳文」)
./scripts/gtl.sh warnings -in games/<game>/ja.jsonl
./scripts/gtl.sh review -in games/<game>/ja.jsonl -fixes games/<game>/fixes.tsv

# 4. Mod を作って入れる (ゲームフォルダに書き込むのはここだけ)
./scripts/gtl.sh <ビルドコマンド> ...          # → dist/<game>
./scripts/gtl.sh install -game <game>
./scripts/gtl.sh uninstall -game <game>      # 外す (上書きしたファイルは元に戻る)

# 5. 訳文を原文抜きの TSV にする (保存・受け渡し用)
./scripts/gtl.sh pack -in games/<game>/ja.jsonl -out games/<game>/ja.tsv
```

## 設定 (gtl.yaml)

```yaml
source_lang: en
target_lang: ja                 # ja / ko / zh-Hans など
llm:
  provider: plamo               # plamo / openai / anthropic
  url: http://127.0.0.1:8000
  model: plamo-3-translate
  # api_key_env: ANTHROPIC_API_KEY   # API キーは環境変数で渡す
fonts:
  # file: fonts/MPLUS1p-Bold.ttf     # ゲームに同梱するフォント
  # system: [Yu Mincho, Meiryo]      # OS のフォントを使うプラグイン向け
```

詳しくは [gtl.example.yaml](gtl.example.yaml)。ゲームの場所が Steam の標準でなければ `games/<game>/game.local.yaml` に `install_dir` を書く。

## 翻訳サーバー (ローカルの PLaMo)

```bash
docker compose up -d llm                     # NVIDIA GPU (127.0.0.1:8000)
docker compose --profile cpu up -d llm-cpu   # GPU が無い場合 (遅い)
docker compose down
```

モデルの取得方法は [docs/SETUP.md](docs/SETUP.md)。

## gtl のコマンド

`./scripts/gtl.sh <コマンド> -h` で各オプションが見られる。

| コマンド | 内容 |
|---|---|
| `ping` | 翻訳モデルの疎通確認と 1 文翻訳 |
| `translate` | JSONL を一括翻訳 (翻訳メモリ・用語集・話者設定・検証と再試行つき) |
| `warnings` / `recheck` / `review` | 警告の一覧 / モデルを呼ばずに検証し直す / 手修正 (fixes.tsv) の取り込み |
| `speakers-check` | 訳文の口調 (一人称・語尾) がキャラクター表と合わない行の一覧 ([docs/CHARACTERS.md](docs/CHARACTERS.md)) |
| `pack` / `unpack` | 原文を除いた訳文 TSV を作る / 抽出した原文に訳文 TSV を合わせる |
| `install` / `uninstall` | `dist/<game>` の Mod をゲームに入れる / 外す |
| (ゲーム固有の拡張) | 拡張があれば `gtl help` の「ゲーム固有のコマンド」に出る (抽出・ビルドなど) |

## フォルダ

| 場所 | 中身 | git |
|---|---|---|
| `cmd/gtl/`, `internal/` | gtl (Go) の共通部分。コマンドを作る部品は `internal/cli` | ○ |
| `scripts/` | `gtl.sh` (gtl をコンテナで実行)、`go.sh` (go をコンテナで実行)、`mage.sh` (開発用のタスクを動かす入口) | ○ |
| `magefiles/`, `mage.go`, `internal/devtools/` | 開発用のタスク ([mage](https://magefile.org/)): lint・公開して問題ないかの確認・データ用リポジトリの操作・Mod の保存 | ○ |
| `cmd/gtl/game_*.go`, `internal/adapter/`, `plugins/`, `magefiles/game_*.go` | ゲーム固有の拡張 (コマンド・データ形式の読み書き・プラグイン・組み立て) | × (データ用リポジトリ) |
| `games/<game>/` | ゲームごとの設定・翻訳結果・訳文・用語集・手修正・調査メモ・ゲーム固有のツール | × (データ用リポジトリ) |
| `docs/private/` | 個人用のメモ | × (データ用リポジトリ) |
| `models/`, `work/`, `dist/` | 翻訳モデル、作業用 (抽出結果・翻訳メモリ・退避)、作った Mod | × |

Claude Code などのコーディングエージェントで作業する場合の決まりは [CLAUDE.md](CLAUDE.md)。
公開しない個人用のメモは `docs/private/` に置く (git 管理外。データ用リポジトリには保存される)。

## ゲーム固有の拡張

ゲームごとの抽出・ビルドは、gtl に「ゲーム固有のコマンド」として足す。ファイルがあればビルドに入り、無ければ共通のコマンドだけで動く。

```go
// cmd/gtl/game_example.go (データ用リポジトリで管理する)
package main

import "github.com/Torkkun/game-translate/internal/cli"

func init() {
	cli.Register("example-extract", "Example の原文を取り出す", cmdExampleExtract)
	cli.Register("example-build", "Example の Mod を dist/example に作る", cmdExampleBuild)
}
```

- 原文は TranslationUnit の JSONL (`internal/unit`) にして `gtl translate` に渡す。ビルドは訳文の JSONL を読んで `dist/<game>` に Mod 一式を作る
- ゲームのデータ形式の読み書きは `internal/adapter/<game>/` に置き、往復テスト (読む → 書く → 同じバイト列) を付ける
- 訳文の言語・フォントは `cli.AddLangFlags` で gtl.yaml から読む。ゲームの場所は `games/<game>/game.yaml` (`internal/gameconf`)
- エンジンごとの定番の方法は [docs/ENGINES.md](docs/ENGINES.md)

## 開発用のタスク (mage)

```bash
./scripts/mage.sh              # タスクの一覧 (Go が無い PC でも動く: コンテナで組み立てて手元で動かす)
./scripts/mage.sh lint:all     # CI と同じ確認 (変更したら必ず通す)
./scripts/mage.sh check        # 公開して問題ないか
go run mage.go lint:all        # Go がある PC (Windows で WSL を使わない場合など) は直接でもよい
```

タスクの中のツール (golangci-lint・shellcheck・gitleaks など) は、Dev Container の中ならそのまま、外ならその都度 Docker のコンテナで動く。
どちらでも手元に要るのは Docker・git・gh だけ。

## ライセンス

ツールは [MIT License](LICENSE)。訳文の扱いや、同梱しないもの (翻訳モデルなど) のライセンスは [docs/LICENSES.md](docs/LICENSES.md)。
