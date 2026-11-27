# CLAUDE.md

このリポジトリで作業する Claude (やほかのコーディングエージェント) 向けの決まりごと。
人向けの説明は [README.md](README.md)、エンジンごとの手順は [docs/ENGINES.md](docs/ENGINES.md)、新しいゲームの調べ方は [docs/RESEARCH.md](docs/RESEARCH.md)、口調のずれを防ぐキャラクター表は [docs/CHARACTERS.md](docs/CHARACTERS.md)、翻訳エンジンの使い分けとエージェントが直接訳す流れは [docs/TRANSLATION.md](docs/TRANSLATION.md)。

過去のつまずきと対策は [docs/LESSONS.md](docs/LESSONS.md)。新しいゲームに取りかかる前と、全文翻訳の後に読む。

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
   `games/<game>/調査レポート.md` (git 管理外、データ用リポジトリに保存) にまとめてユーザーに方式を確認する。エンジンごとの定番の方法は [docs/ENGINES.md](docs/ENGINES.md)。
   方式が決まったら、決まったことを `games/<game>/README.md` に移し、調査レポートは消す (リポジトリには決まったことだけを残し、作業の記録は残さない)
2. **抽出**: ゲーム固有の拡張として `internal/adapter/<game>/` に読み書きのコードと往復テスト (読む → 書く → 同じバイト列) を作り、
   `cmd/gtl/game_<game>.go` の init で `cli.Register` してコマンドにする。
   `gtl <game>-extract` で原文を TranslationUnit の JSONL (`work/<game>/source.jsonl`) にする。
   書式記号 (`{0}`, `<b>`, `\n` など) は訳文でも保たれるよう、検証 (`internal/check`) で拾える形にする。
   ゲームが内部の ID・キーとしても使う文字列 (照合や ID の組み立てに使う表示名・音声の名前など) は grep で探し、訳さない (用語集で英語に固定するか、対象から外す)
3. **キャラクター表と試験翻訳**: [docs/CHARACTERS.md](docs/CHARACTERS.md) の手順で、話者の割り当て (`context.speaker`) とキャラクター表 (`characters.yaml`) を作り、ユーザーに確認する。
   口調は表で決め、性別だけで決めつけない。`gtl translate -limit 50 -speakers ...` で数十行訳してユーザーに見せ、用語集 (`glossary.yaml`)・表・文体を決める。
   用語集には、`gtl consistency -in work/<game>/source.jsonl -min-names 3` の「固有名詞の候補」(人名・地名・艦名) と、よく出る UI の語を全文翻訳の前に入れる
4. **全文翻訳**: 文の種類ごとに訳し方を決め、`gtl.yaml` の `routes` に書く ([docs/TRANSLATION.md](docs/TRANSLATION.md))。
   基本は、ストーリー・会話はエージェント (Claude) が直接訳し (`agent`)、UI・アイテム名・大量の説明文は翻訳モデル (PLaMo) で訳して自動チェックにかける。
   `gtl translate -speakers games/<game>/characters.yaml` (中断しても続きから) でモデルの分を訳し、
   エージェントの分は `gtl batch-export` → 作業ファイルの `target` を埋める → `gtl batch-import` で取り込む。
   `gtl warnings`・`gtl speakers-check`・`gtl consistency` で確かめ、翻訳モデルの訳は UI の短い行を中心に見直して ([docs/TRANSLATION.md](docs/TRANSLATION.md#訳した後の点検))、`fixes.tsv` + `gtl review` で直す。
   人が口調を直したら、多かった言い方をキャラクター表の `avoid` / `avoid_endings` に足す
5. **ビルド**: `gtl <game>-build` で `dist/<game>` に Mod 一式を作る (ゲームフォルダには触らない)。手順が多いゲームは `magefiles/game_*.go` に `Package` のタスクとしてまとめる
6. **導入**: ユーザーに確認してから `gtl install -game <game>`。入れるのは最新の訳文から組み立て直した `dist/<game>` だけにし、入れた主なファイルの sha256 を確かめる。
   実機で表示 (字幕・選択肢・メニュー・長い台詞・フォントの違う画面) と主な操作を確かめてもらう
7. **保存**: `gtl pack` で `games/<game>/<言語>.tsv` を更新し、`./scripts/mage.sh release <game> <版> "メモ"` で訳文と Mod をデータ用リポジトリに保存する。ゲームごとの方式・入れ方と外し方・作り方・訳文の元・決めたこと・既知の制限は `games/<game>/README.md` (データ用リポジトリ) に書き、データ用リポジトリのトップの Mod 一覧 (`.github/README.md`) を更新する。この README の「対応実績」には 1 行だけ足す

### 訳文を確かめるときの決まり

- 会話・台詞のある行は、話者とキャラクター表があるなら必ず `gtl speakers-check -summary` を通し、件数をユーザーに報告する。
  キャラクター表より前に訳した分は `-masculine` も付けて、表に書いていない男言葉 (あいつ・お前・〜か？・〜だろ・〜だな・〜ぜ・命令形) を点検する。
  乱暴な口調の人物は表の `allow` で許す ([docs/CHARACTERS.md](docs/CHARACTERS.md#男言葉の点検))
- 口調を決めるのはキャラクター表。勝気・荒っぽい人物でも、どこまで許すか (例: 「あんた」「〜だろ」は可、「俺」「〜ぜ」は不可) を表に書く
- 大量に訳し直す前に、件数と代表例をユーザーに見せて判断を待つ。手修正済みの行 (`fixes.tsv`) は上書きしない
- 確認の言い方を足したら、正しい訳で誤検知が出ないことをテストで確かめる
- 訳文は gtl の外 (スクリプト・JSON など) で直さず、`fixes.tsv` + `gtl review` で取り込む (手修正の書式記号のずれも警告される)。
  原文に無い書式指定 (`%s` など) は、ほかの言語の訳にあっても足さない
- 同じ原文の訳がぶれていないかを `gtl consistency` で確かめる。人名・UI の語の不統一は用語集に足してから直す

## コードの決まり

- コメント・ログ・エラーメッセージは日本語 (既存のコードに合わせる)
- 共通部分 (このリポジトリ) はゲーム固有の拡張が無くてもビルド・テストが通るようにする。共通部分からゲーム固有のパッケージを import しない
- CI は `ci.yml` (親) から `ci-*.yml` (子) を呼ぶ形。リリースの流れは [docs/RELEASE.md](docs/RELEASE.md)
- 開発用の作業はシェルスクリプトではなく mage のタスク (`magefiles/`) に書く。テストしたい処理は `internal/devtools` に置く。シェルに残すのは、コンテナを起動する入口 (`scripts/gtl.sh`・`go.sh`・`mage.sh`) だけ
- 訳文の言語・フォントは決め打ちせず `config.Config` (gtl.yaml) から読む。ゲーム固有の値 (枠の言語、ファイル名) はオプションにする
- PC ごとに違う値 (ゲームの場所など) は `games/<game>/game.local.yaml` (git 管理外) に書く
