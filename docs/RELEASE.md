# CI とリリースの流れ

## CI (ツール)

`main` への push と pull request で [.github/workflows/ci.yml](../.github/workflows/ci.yml) が動く。
親の `ci.yml` が、種類ごとの子ワークフローを呼ぶ。

| 子ワークフロー | 内容 |
|---|---|
| `ci-guard.yml` | 公開して問題ないか: ゲーム由来のファイル・訳文・個人のパス (`mage check`)、秘密情報 (gitleaks) |
| `ci-go.yml` | gofmt、`go mod verify`、go vet、golangci-lint ([.golangci.yml](../.golangci.yml))、`go test -race`、ビルド (gtl と mage のタスク) |
| `ci-devcontainer.yml` | Dev Container のイメージが組み立てられ、普段使うツールが入っているか |
| `ci-shell.yml` | shellcheck (入口の `scripts/*.sh`)、`compose.yaml` の検査 |

同じ確認は手元でも `./scripts/mage.sh lint:all` (全部) / `./scripts/mage.sh lint:go` (種類を指定) でできる (ツールは Dev Container の中ならそのまま、外ならコンテナで動く)。
手元ではゲーム固有の拡張 (あれば) も一緒に確かめる (Python の補助ツールは ruff、プラグインは dotnet format、フックの build.sh は shellcheck)。
CI は共通部分だけを確かめる (拡張が無くてもビルド・テストが通ること)。
子ワークフローを足すときは `ci-<種類>.yml` を `on: workflow_call` で作り、`ci.yml` の `jobs` に足す。

## ツール (gtl) のリリース (メンテナー向け)

```bash
git tag v0.1.0
git push origin v0.1.0
```

[release.yml](../.github/workflows/release.yml) が CI を通してから、Windows / Linux / macOS 向けの gtl をビルドし、
zip と SHA256SUMS.txt を GitHub Release に添付する。版の付け方は [セマンティック バージョニング](https://semver.org/lang/ja/) に従う
(設定ファイルやコマンドの互換が崩れる変更はメジャーを上げる)。

## Mod・訳文の更新

Mod を作るにはゲーム本体のファイルが必要で、GitHub Actions のサーバーにはゲームが無いため、**ビルドは手元で行う**。
Mod 本体と訳文はゲーム由来のデータなので、このリポジトリではなく**データ用リポジトリ**に保存する。
データ用リポジトリは使う人が GitHub などに**非公開**で作る (中身を公開・配布してはいけない)。

### データ用リポジトリのしくみ

作業フォルダはこのリポジトリと同じで、git の管理情報だけを `.data.git` に分ける。
追跡するのは [scripts/data.files](../scripts/data.files) に書いたもの
(`games/<game>/` の設定・翻訳結果・訳文・用語集・手修正・調査メモ、ゲーム固有の拡張のコード、`docs/private/` など) だけで、
このリポジトリの `.gitignore` は同じものを外している。ファイルの置き場所は変わらないので、ツールはそのまま使える。

| 保存するもの | 保存先 |
|---|---|
| 訳文・翻訳結果・用語集・手修正など (文字のデータ) | データ用リポジトリの git (履歴が残る) |
| Mod 一式 (`dist/<game>`)・翻訳メモリ (`work/<game>/tm.sqlite`) | データ用リポジトリの Release (ゲーム・版ごとの zip、1 ファイル 2GB まで) |

```bash
gh repo create <user>/<データ用リポジトリ> --private                             # 最初に 1 回 (非公開で作る)
./scripts/mage.sh data:init https://github.com/<user>/<データ用リポジトリ>.git  # 初回 (別の PC でも同じ)
./scripts/mage.sh data:status                                                # 変更の一覧 (data:git "log --oneline" で git のコマンドも使える)
./scripts/mage.sh data:save "手修正 20 件"                                    # 訳文だけ保存 (commit + push)
```

VS Code などの git の画面はこのリポジトリだけを扱う。データ用リポジトリは `./scripts/mage.sh data:...` で操作する。

### 更新の流れ

1. 訳文を直す (`gtl review` / `gtl translate -retry-warned`)
2. 手元で Mod を作る (`./scripts/mage.sh package:<game>` か `gtl <game>-build`)
3. 実機で確認する (`gtl install` でゲームフォルダに入れる。何が入るかを確かめてから)
4. 保存する

   ```bash
   ./scripts/gtl.sh pack -in games/<game>/ja.jsonl -out games/<game>/ja.tsv
   ./scripts/mage.sh release <game> v1.1 "メモ"
   ```

   訳文などを commit してタグ `<game>-v1.1` を付け、Mod 一式と翻訳メモリの zip を Release `<game>-v1.1` に添付する。

版はゲームごとに `v<メジャー>.<マイナー>` で付ける (Mod の作りが変わったらメジャー、訳の修正はマイナー)。

### 公開して問題ないかの確認

CI の Guard ([ci-guard.yml](../.github/workflows/ci-guard.yml)) が毎回、
ゲーム由来のファイル・訳文・大きなファイル・個人の環境のパス (`mage check`) と、
API キーなどの秘密情報 (gitleaks、履歴を含む) が公開リポジトリに入っていないかを確かめる。
