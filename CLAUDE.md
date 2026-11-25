# CLAUDE.md

このリポジトリで作業する Claude (やほかのコーディングエージェント) 向けの決まりごと。
人向けの説明は [README.md](README.md)、エンジンごとの手順は [docs/ENGINES.md](docs/ENGINES.md)、新しいゲームの調べ方は [docs/RESEARCH.md](docs/RESEARCH.md)。

## 何をするリポジトリか

Steam などの PC ゲームを、LLM の機械翻訳 + 手修正で別の言語 (主に日本語) にする Mod を作るツール `gtl` (Go) の共通部分。
ゲームごとの抽出・ビルドは「ゲーム固有の拡張」(`cmd/gtl/game_*.go`・`internal/adapter/<game>/`・`plugins/`・`games/<game>/`) で、
このリポジトリには入れずデータ用リポジトリで管理する ([README](README.md#ゲーム固有の拡張))。訳す言語・翻訳モデル・フォントは `gtl.yaml` で切り替える ([gtl.example.yaml](gtl.example.yaml))。

## 必ず守ること

1. **ゲームフォルダは読み取り専用で扱う。** 抽出・調査・ビルドはゲームのファイルを読むだけにする。
   ゲームフォルダに書き込むのは `gtl install` / `gtl uninstall` だけで、その前に**必ずユーザーに確認する**。
   ほかの方法 (コピー、上書き、設定ファイルの編集) でゲームフォルダを変えるときも同じ。
2. **上書きする前に退避する。** `gtl install` は上書きするファイルを `work/<game>/backup/` に退避し、
   入れたものを `gtl-install.json` に記録する。手で入れる手順を作るときも、元に戻す方法 (退避先・消すファイル) を先に決めて書いておく。
3. **ゲーム由来のデータを git に入れない。** 原文 (ゲームの英文) を含む `*.jsonl`、ゲームから取り出したファイル、
   ゲームの素材から作った Mod 本体 (アーカイブ・パッチ・書き換えたデータファイルなど) は .gitignore で外してある。
   訳文 (`<言語>.tsv`)・用語集・話者設定・手修正、ゲームごとの調査メモ、ゲーム固有の拡張 (コード・プラグイン・組み立てスクリプト)、
   個人用のメモ (`docs/private/`) もこのリポジトリには入れず、
   データ用リポジトリ (`mage data:save`、対象は `scripts/data.files`。非公開で作る) で管理する。このリポジトリに入れるのはツール・手順・設定だけ。
   `mage check` (CI の Guard) が、これらや個人の環境のパスが入っていないかを確かめる。
   新しいファイルを足すときは、ゲームの文章や素材がそのまま入っていないか確かめる。
4. **ほかの人の訳を使うときはライセンスを確かめて記録する。** 既存のファン翻訳を土台にする場合は、
   ライセンス文を `games/<game>/LICENSE-*.txt` に置いて組み立てた Mod にも同梱し、出典を拡張の説明に書く ([docs/LICENSES.md](docs/LICENSES.md))。ライセンスが不明なものは使わない。
5. **インストールが必要なものはコンテナの中で動かす。** ホスト (WSL / Windows) に Go・.NET・Python のライブラリなどを入れない。
   普段の開発に使うツールは Dev Container (`.devcontainer/Dockerfile`) に入れ、Mod の組み立てのときだけ使うもの (.NET・MinGW・Python など) は
   mage のタスクがその都度コンテナで動かす。翻訳サーバーは `compose.yaml`。Dev Container の外でも `./scripts/go.sh`・`gtl.sh`・`mage.sh` で動く。
   ツールの版は `.devcontainer/Dockerfile`・`magefiles/magefile.go`・CI (`.github/workflows/`) で同じにする
6. **成果物は作った場所に関わらず保存する。** リポジトリの外 (別のフォルダ、ゲームの Mod フォルダなど) で Mod や訳を作ったときも、
   できた時点でツール・手順はこのリポジトリに、訳文・ゲーム由来のデータは `./scripts/mage.sh data:save "メモ"`、Mod 本体は `./scripts/mage.sh release <game> <版> "メモ"` で保存する。
7. **公開する文書は対外向けに書く。** README・`docs/` (`docs/private/` を除く)・CLAUDE.md は、外部の人が読む前提で書く。
   個人の環境・作業の経緯・自分のリポジトリやアカウントの話は `docs/private/` (git 管理外) に書く。
8. **API キーをファイルに書かない。** `gtl.yaml` には環境変数の名前 (`api_key_env`) だけを書く。

## よく使うコマンド

```bash
./scripts/mage.sh lint:all                                 # 変更したら必ず通す (CI と同じ確認 + 手元にあるゲーム固有の拡張の確認)
./scripts/mage.sh                                          # 開発用のタスクの一覧 (magefiles/。Go がある PC は go run mage.go でもよい)
./scripts/go.sh test ./internal/llm                        # Go のテストだけ
./scripts/gtl.sh ping                                      # 翻訳モデルの確認
./scripts/gtl.sh <コマンド> -h                              # 各コマンドのオプション
```

## 新しいゲームに対応する流れ

1. **調査 (読み取りのみ)**: [docs/RESEARCH.md](docs/RESEARCH.md) の手順で、エンジン、テキストの場所と形式、フォント、既存の翻訳 (公式・ファン) を調べ、
   `games/<game>/調査レポート.md` (git 管理外、データ用リポジトリに保存) にまとめてユーザーに方式を確認する。エンジンごとの定番の方法は [docs/ENGINES.md](docs/ENGINES.md)
2. **抽出**: ゲーム固有の拡張として `internal/adapter/<game>/` に読み書きのコードと往復テスト (読む → 書く → 同じバイト列) を作り、
   `cmd/gtl/game_<game>.go` の init で `cli.Register` してコマンドにする。
   `gtl <game>-extract` で原文を TranslationUnit の JSONL (`work/<game>/source.jsonl`) にする。
   書式記号 (`{0}`, `<b>`, `\n` など) は訳文でも保たれるよう、検証 (`internal/check`) で拾える形にする
3. **試験翻訳**: `gtl translate -limit 50` で数十行訳してユーザーに見せ、用語集 (`glossary.yaml`)・話者設定・文体を決める
4. **全文翻訳**: `gtl translate` (中断しても続きから)。`gtl warnings` で警告を見て、`fixes.tsv` + `gtl review` で直す
5. **ビルド**: `gtl <game>-build` で `dist/<game>` に Mod 一式を作る (ゲームフォルダには触らない)。手順が多いゲームは `magefiles/game_*.go` に `Package` のタスクとしてまとめる
6. **導入**: ユーザーに確認してから `gtl install -game <game>`。実機で表示を確かめてもらう
7. **保存**: `gtl pack` で `games/<game>/<言語>.tsv` を更新し、`./scripts/mage.sh release <game> <版> "メモ"` で訳文と Mod をデータ用リポジトリに保存する。詳しい状態は `docs/private/` のメモに書き、README の「対応実績」には 1 行だけ足す

## コードの決まり

- コメント・ログ・エラーメッセージは日本語 (既存のコードに合わせる)
- 共通部分 (このリポジトリ) はゲーム固有の拡張が無くてもビルド・テストが通るようにする。共通部分からゲーム固有のパッケージを import しない
- CI は `ci.yml` (親) から `ci-*.yml` (子) を呼ぶ形。リリースの流れは [docs/RELEASE.md](docs/RELEASE.md)
- 開発用の作業はシェルスクリプトではなく mage のタスク (`magefiles/`) に書く。テストしたい処理は `internal/devtools` に置く。シェルに残すのは、コンテナを起動する入口 (`scripts/gtl.sh`・`go.sh`・`mage.sh`) だけ
- 訳文の言語・フォントは決め打ちせず `config.Config` (gtl.yaml) から読む。ゲーム固有の値 (枠の言語、ファイル名) はオプションにする
- PC ごとに違う値 (ゲームの場所など) は `games/<game>/game.local.yaml` (git 管理外) に書く
