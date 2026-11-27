# 翻訳エンジンの使い分け

gtl は文の種類ごとに翻訳エンジンを選べる。訳文の質と手間・費用の兼ね合いで、次の分け方を基本にする。

| 文の種類 (`context.kind`) | 向いている訳し方 | 理由 |
|---|---|---|
| ストーリー・会話 (`dialogue` など) | エージェント (Claude など) が直接訳す | キャラクター表・話者・前後の会話をまとめて読めるので、口調と流れがそろう。量はゲーム全体の一部 |
| UI・アイテム名・大量の説明文 (`ui` `item` など) | 翻訳モデル (PLaMo 3 Translate など) で一括 → 自動チェック → 見直し | 短い・定型の文が多く、ローカルで速く、費用がかからない。ただし短い語ほど意味を取り違えやすい (下の「訳した後の点検」) |

どの種類をどう訳すかはゲームごとに違う (台詞が少ないゲーム、説明文が物語の中心のゲームもある)。試験翻訳の結果を見て決める。

翻訳モデルにキャラクターごとの口調を指示しても、モデルはほとんど従わない (話者ごとに別の翻訳サーバーを立てても変わらない)。
口調が大事な行は、エージェントが訳すほうに回す。

## 設定 (gtl.yaml)

```yaml
llm:                 # 既定のエンジン ("default")
  provider: plamo
models:              # 名前付きのエンジン (routes で使う)
  claude:
    provider: anthropic   # model を省くと既定の Claude のモデル
    api_key_env: ANTHROPIC_API_KEY
routes:              # 文の種類 (context.kind) → エンジンの名前
  dialogue: agent    # gtl translate では訳さず、エージェントが batch-export / batch-import で訳す
  narration: claude  # Claude API で 1 行ずつ (文脈付き) 訳す
  "*": default       # そのほかは llm
```

- `default` は `llm` のエンジン、`agent` は「gtl translate では訳さない」という意味の予約名
- `routes` に無い種類は `"*"` の行、それも無ければ `default` で訳す
- `gtl translate -no-routes` で、routes を無視してすべての行を `llm` で訳せる
- 行の種類は抽出 (ゲーム固有の `<game>-extract`) で `context.kind` に入れる。種類が付いていないゲームでは routes の `"*"` だけが効く

## エージェントが直接訳す流れ

1. `gtl translate ...` で、エージェントに回さない行を訳す (回した行の数がログに出る)
2. `gtl batch-export -in work/<game>/source.jsonl -speakers games/<game>/characters.yaml -glossary games/<game>/glossary.yaml`
   で、訳の無いエージェント向けの行を、シーン順の作業ファイル (`<翻訳結果>.batches/batch_0001.json` …) に書き出す。
   各ファイルには、訳し方の指示・このファイルに出てくる人物のキャラクター表・用語集・行 (ID・話者・原文・空の訳) が入る
   - `-all` は routes に関係なく訳の無い行をすべて、`-warned` は機械翻訳で警告が残った行も書き出す (訳し直し用)
   - `-size` で 1 ファイルの行数 (既定 40) を変える。シーンの途中ではなるべく切らない
3. エージェントが作業ファイルを 1 つずつ読み、`lines[].target` を埋める (下の「エージェントへの注意」)
4. `gtl batch-import` (batch-export と同じオプション) で翻訳結果に取り込む。取り込んだ行は機械翻訳と同じ扱い (`status: machine`) で、
   書式記号・用語集・キャラクター表の口調のチェックがかかる。人が確認した行 (`reviewed` / `locked`) は上書きしない
5. いつもどおり `gtl warnings` / `gtl speakers-check` で確かめ、`fixes.tsv` + `gtl review` で直す

途中でやめても、取り込んだ行は訳済みになるので、もう一度 batch-export すれば残りだけが書き出される。

### エージェントへの注意

- ファイルの `instructions` に従う。とくに、話者のある行はキャラクター表の一人称・二人称・話し方に合わせ、話者の無い台詞では性別の強く出る言い方を避ける
- 前後の行 (同じ `scene`) を読んでから訳す。1 行ずつ機械的に訳さない
- `id`・`source` などは変えず、`target` だけを書く。JSON として壊さない (ファイルを書いたら読み直して確かめる)
- 1 回に訳すのは 1〜数ファイルにして、終わったら batch-import で取り込んで警告を確かめてから次に進む。最初の数ファイルはユーザーに見せて口調を確認する
- 訳文の質はモデルや設定によって変わる。実際のゲームで表示を確かめてもらうまで「完成」とは言わない

## 訳した後の点検

翻訳モデルの訳は、全文翻訳の後に一度見直す。見直しで直した行の多くは、UI の短い語・固有名詞・慣用句だった ([LESSONS.md](LESSONS.md#4-翻訳-機械翻訳の直訳取り違え))。

1. **全文翻訳の前**: `gtl consistency -in work/<game>/source.jsonl -glossary games/<game>/glossary.yaml -min-names 3` の「固有名詞の候補」を見て、
   人名・地名・艦名などの訳 (英語のまま残すかどうかも) を用語集に書く。UI のよく出る語 (Resume・Exit・Show など) も、画面での意味で用語集に書く
2. **抽出のとき**: UI の行には `context.kind: ui` と、画面・用途の説明 (`context.note`) を付ける。翻訳モデルにもエージェントにも渡る
3. **訳した後**: `gtl consistency -in games/<game>/ja.jsonl` で、同じ原文なのに訳が違う組を一覧して統一する
4. エージェントが、短い UI の行・カタカナだけの訳・慣用句を含む行を見直し、直す訳を `fixes.tsv` に書く。
   原文・今の訳・直した訳・理由の表をユーザーに見せてから取り込む
5. `gtl review` で取り込む。手修正の行も書式記号 (`%s`・`\n` など) と改行の数を検証し直し、ずれていれば警告として残す
