package translate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/llm"
	"github.com/Torkkun/game-translate/internal/unit"
)

// エージェント (Claude など) が直接訳すための受け渡し。
//
// gtl translate はモデルに 1 行ずつ訳させるが、ストーリーや会話はキャラクター表と前後の文脈をまとめて渡せる
// エージェントが訳したほうが口調がそろう。BatchExport が訳す行をシーン順に「バッチ」(JSON) にまとめ、
// エージェントが各行の target を埋め、BatchImport が取り込んで、ほかの訳と同じ自動チェックにかける。

// BatchInstructions はバッチの先頭に入れる、訳し方の指示 (エージェント向け)。
const BatchInstructions = `このファイルはゲームの翻訳の作業ファイルです。lines の各行の source を target_lang に訳し、target に書いてください。
- 行はゲームの中の順番に並んでいます。前後の行を読んで、会話の流れ・話者・場面に合う訳にしてください
- speaker が書いてある行は、characters にあるその人物の口調 (一人称・二人称・話し方・使わない言い方) に必ず合わせてください。
  speaker が無い台詞は、原文から話者がはっきりしない限り、性別が強く出る言い方 (俺・僕・あたし、〜だぜ・〜わよ など) を避けてください
- glossary の語は指定の訳にしてください (訳が原語と同じものは原語のまま残す)
- 書式記号 ({0} %s <b> </1> [p] \n など) と改行の数は原文のまま残してください
- 訳だけを書き、注釈・ルビ・括弧書きの説明は足さないでください。id・source などほかの項目は変えないでください
- 訳せない行は target を空のままにしてかまいません (残った行は gtl batch-import が一覧にします)`

// Batch はエージェントに渡す 1 つの作業ファイル。
type Batch struct {
	Instructions string            `json:"instructions"`
	SourceLang   string            `json:"source_lang"`
	TargetLang   string            `json:"target_lang"`
	Form         string            `json:"form,omitempty"`
	GameNote     string            `json:"game_note,omitempty"`  // 多くの行に共通の説明 (ゲームの紹介など)
	Characters   map[string]string `json:"characters,omitempty"` // このバッチに出てくる話者 → キャラクター表の説明
	Glossary     []llm.Term        `json:"glossary,omitempty"`   // このバッチに出てくる用語
	Lines        []BatchLine       `json:"lines"`
}

// BatchLine はバッチの 1 行。
type BatchLine struct {
	ID      string `json:"id"`
	Scene   string `json:"scene,omitempty"`
	Speaker string `json:"speaker,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Note    string `json:"note,omitempty"` // game_note と違うときだけ
	Source  string `json:"source"`
	Target  string `json:"target"`
}

// BatchOptions はバッチの作り方。
type BatchOptions struct {
	SourceLang, TargetLang, Form string
	Size                         int // 1 バッチのおおよその行数 (シーンの途中ではなるべく切らない)
	Speakers                     map[string]Speaker
	Glossary                     *glossary.Glossary
}

// BatchExport は訳す行 (原文の順) をバッチに分ける。
func BatchExport(units []unit.Unit, o BatchOptions) []Batch {
	if o.Size <= 0 {
		o.Size = 40
	}
	if o.Glossary == nil {
		o.Glossary = &glossary.Glossary{}
	}
	note := commonNote(units)
	// シーンの切れ目で区切る。1 シーンが大きければ途中でも区切る
	var batches [][]unit.Unit
	var cur []unit.Unit
	for i, u := range units {
		newScene := i > 0 && u.Context.Scene != units[i-1].Context.Scene
		if len(cur) >= o.Size || (newScene && len(cur) >= o.Size*3/4) {
			batches = append(batches, cur)
			cur = nil
		}
		cur = append(cur, u)
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}

	out := make([]Batch, 0, len(batches))
	for _, us := range batches {
		b := Batch{Instructions: BatchInstructions, SourceLang: o.SourceLang, TargetLang: o.TargetLang, Form: o.Form, GameNote: note}
		chars := map[string]string{}
		terms := map[string]llm.Term{}
		for _, u := range us {
			l := BatchLine{ID: u.ID, Scene: u.Context.Scene, Speaker: u.Context.Speaker, Kind: u.Context.Kind, Source: u.Source}
			if u.Context.Note != note {
				l.Note = u.Context.Note
			}
			if sp, ok := o.Speakers[u.Context.Speaker]; ok {
				name := sp.Name
				if name == "" {
					name = u.Context.Speaker
				}
				l.Speaker = name
				chars[name] = sp.Describe()
			}
			for _, t := range glossary.ToLLM(o.Glossary.Match(u.Source)) {
				terms[t.Source] = t
			}
			b.Lines = append(b.Lines, l)
		}
		if len(chars) > 0 {
			b.Characters = chars
		}
		for _, t := range terms {
			b.Glossary = append(b.Glossary, t)
		}
		sort.Slice(b.Glossary, func(i, j int) bool { return b.Glossary[i].Source < b.Glossary[j].Source })
		out = append(out, b)
	}
	return out
}

// commonNote は半分以上の行に共通の説明を返す (無ければ空)。
func commonNote(units []unit.Unit) string {
	count := map[string]int{}
	for _, u := range units {
		count[u.Context.Note]++
	}
	best, n := "", 0
	for k, v := range count {
		if v > n || (v == n && k < best) {
			best, n = k, v
		}
	}
	if n*2 < len(units) {
		return ""
	}
	return best
}

// WriteBatches はバッチを dir/batch_0001.json … に書く。既にあるバッチのファイルは消してから書く。
func WriteBatches(dir string, batches []Batch) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	old, _ := filepath.Glob(filepath.Join(dir, "batch_*.json"))
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return nil, err
		}
	}
	var paths []string
	for i, b := range batches {
		p := filepath.Join(dir, fmt.Sprintf("batch_%04d.json", i+1))
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false) // <b> などの書式記号をそのまま読めるように
		enc.SetIndent("", "  ")
		if err := enc.Encode(b); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// ReadBatchTargets は dir のバッチから、訳の書いてある行の ID → 訳文を読む。
func ReadBatchTargets(dir string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "batch_*.json"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s にバッチ (batch_*.json) がありません", dir)
	}
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var batch Batch
		if err := json.Unmarshal(b, &batch); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		for _, l := range batch.Lines {
			if strings.TrimSpace(l.Target) != "" {
				out[l.ID] = l.Target
			}
		}
	}
	return out, nil
}
