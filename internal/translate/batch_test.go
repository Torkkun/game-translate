package translate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/llm"
	"github.com/Torkkun/game-translate/internal/unit"
)

func TestBatchExportImport(t *testing.T) {
	note := "From a detective game."
	var units []unit.Unit
	for i, sc := range []string{"s1", "s1", "s1", "s2", "s2", "s3"} {
		units = append(units, unit.Unit{
			ID: string(rune('a' + i)), Source: "Hello Alice " + sc,
			Context: unit.Context{Scene: sc, Speaker: "ALIC", Note: note},
		})
	}
	units[5].Context.Note = "A menu label."
	units[5].Context.Speaker = ""
	g, _ := glossary.New([]glossary.Term{{Source: "Alice", Target: "Alice"}})
	sp := map[string]Speaker{"ALIC": {Name: "Alice", Gender: "female", FirstPerson: "私"}}

	bs := BatchExport(units, BatchOptions{SourceLang: "en", TargetLang: "ja", Size: 4, Speakers: sp, Glossary: g})
	if len(bs) != 2 || len(bs[0].Lines) != 3 || len(bs[1].Lines) != 3 {
		t.Fatalf("シーンの切れ目で分かれるはず: %d %+v", len(bs), bs)
	}
	b := bs[0]
	if b.GameNote != note || b.Lines[0].Note != "" || b.Lines[0].Speaker != "Alice" || b.Characters["Alice"] == "" || len(b.Glossary) != 1 {
		t.Fatalf("%+v", b)
	}
	if bs[1].Lines[2].Note != "A menu label." {
		t.Fatalf("共通でない説明は行に残す: %+v", bs[1].Lines[2])
	}

	dir := t.TempDir()
	paths, err := WriteBatches(dir, bs)
	if err != nil || len(paths) != 2 {
		t.Fatal(err, paths)
	}
	// エージェントが 1 行だけ訳した
	var got Batch
	data, _ := os.ReadFile(paths[0])
	_ = json.Unmarshal(data, &got)
	got.Lines[1].Target = "こんにちは Alice"
	data, _ = json.Marshal(got)
	if err := os.WriteFile(filepath.Join(dir, "batch_0001.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	tg, err := ReadBatchTargets(dir)
	if err != nil || len(tg) != 1 || tg["b"] != "こんにちは Alice" {
		t.Fatalf("%v %v", err, tg)
	}
}

func TestRouteDefersAgentLines(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.jsonl")
	out := filepath.Join(dir, "out.jsonl")
	us := []unit.Unit{
		{ID: "1", Source: "OK", Context: unit.Context{Kind: "ui"}},
		{ID: "2", Source: "Hello there.", Context: unit.Context{Kind: "dialogue"}},
	}
	if err := unit.WriteJSONL(in, us); err != nil {
		t.Fatal(err)
	}
	f := &fake{}
	r := &Runner{LLM: f, Opt: Options{In: in, Out: out, Workers: 1, Attempts: 1},
		Route: func(u unit.Unit) (tr llm.Translator, agent bool) { return nil, u.Context.Kind == "dialogue" }}
	st, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if st.Deferred != 1 || st.Translated != 1 || len(f.reqs) != 1 || f.reqs[0].Text != "OK" {
		t.Fatalf("%+v %+v", st, f.reqs)
	}
}
