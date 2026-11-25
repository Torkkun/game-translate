package translate

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/llm"
	"github.com/Torkkun/game-translate/internal/tm"
	"github.com/Torkkun/game-translate/internal/unit"
)

// fake は原文を全角にして「訳「…」」で囲んで返す翻訳器。受け取った依頼を記録する。
// (半角英字のままだと検証で「未翻訳に見える」と判定されるため全角にする)
type fake struct {
	mu   sync.Mutex
	reqs []llm.Request
}

func (f *fake) Translate(_ context.Context, r llm.Request) (string, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	f.mu.Unlock()
	return fakeTranslate(r.Text), nil
}

func fakeTranslate(s string) string {
	var b strings.Builder
	b.WriteString("訳「")
	for _, c := range s {
		if c > ' ' && c <= '~' {
			c += 0xFEE0
		}
		b.WriteRune(c)
	}
	b.WriteString("」")
	return b.String()
}

func (f *fake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func writeInput(t *testing.T, dir string, units []unit.Unit) string {
	t.Helper()
	p := filepath.Join(dir, "in.jsonl")
	if err := unit.WriteJSONL(p, units); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunHistoryResumeAndTM(t *testing.T) {
	dir := t.TempDir()
	in := writeInput(t, dir, []unit.Unit{
		{ID: "a1", Source: "First line.", Context: unit.Context{Scene: "a"}},
		{ID: "a2", Source: "Second line.", Context: unit.Context{Scene: "a"}},
		{ID: "a3", Source: "Third line.", Context: unit.Context{Scene: "a"}},
		{ID: "u1", Source: "Ask Gus.", Context: unit.Context{Kind: "ui"}},
		{ID: "u2", Source: "First line.", Context: unit.Context{Kind: "ui"}}, // a1 と同じ原文 → TM
	})
	out := filepath.Join(dir, "out.jsonl")
	g, _ := glossary.New([]glossary.Term{{Source: "Gus", Target: "ガス"}})
	mem, err := tm.Open(filepath.Join(dir, "tm.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	// 1 回目: 2 件だけ訳す
	f := &fake{}
	r := &Runner{LLM: f, TM: mem, Glossary: g, Opt: Options{In: in, Out: out, TargetLang: "ja", Workers: 1, History: 6, Limit: 2, UseTM: true}}
	st, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Translated != 2 || f.count() != 2 {
		t.Fatalf("2 件訳すはず: %+v, 呼び出し %d", st, f.count())
	}

	// 2 回目: 残りを訳す。訳済みは飛ばし、a3 には a1, a2 が履歴として渡る
	f = &fake{}
	r.LLM = f
	r.Opt.Limit = 0
	r.Opt.Workers = 3
	st, err = r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Skipped != 2 {
		t.Fatalf("訳済み 2 件を飛ばすはず: %+v", st)
	}
	if st.FromTM != 1 {
		t.Fatalf("u2 は翻訳メモリから再利用するはず: %+v", st)
	}

	var a3, u1 *llm.Request
	for i := range f.reqs {
		switch f.reqs[i].Text {
		case "Third line.":
			a3 = &f.reqs[i]
		case "Ask Gus.":
			u1 = &f.reqs[i]
		}
	}
	if a3 == nil || len(a3.History) != 2 || a3.History[0].Target != fakeTranslate("First line.") {
		t.Fatalf("a3 の履歴が違う: %+v", a3)
	}
	if u1 == nil || len(u1.Glossary) != 1 || !strings.Contains(u1.Context, "user interface") {
		t.Fatalf("u1 の用語集・文脈が違う: %+v", u1)
	}

	// 出力は入力順・全件・訳あり
	got, err := unit.ReadJSONL(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("出力は 5 件のはずが %d 件", len(got))
	}
	for i, id := range []string{"a1", "a2", "a3", "u1", "u2"} {
		if got[i].ID != id || got[i].Target == "" || got[i].Status != unit.StatusMachine {
			t.Fatalf("出力 %d 件目が違う: %+v", i, got[i])
		}
	}
	// 訳文に「ガス」が無いので用語集の警告が付く
	if len(got[3].Warnings) == 0 {
		t.Fatalf("u1 に用語集の警告が付くはず: %+v", got[3])
	}

	// 3 回目: 原文が変わった行だけ訳し直す
	in2 := writeInput(t, dir, []unit.Unit{
		{ID: "a1", Source: "First line, changed.", Context: unit.Context{Scene: "a"}},
		{ID: "a2", Source: "Second line.", Context: unit.Context{Scene: "a"}},
		{ID: "a3", Source: "Third line.", Context: unit.Context{Scene: "a"}},
		{ID: "u1", Source: "Ask Gus.", Context: unit.Context{Kind: "ui"}},
		{ID: "u2", Source: "First line.", Context: unit.Context{Kind: "ui"}},
	})
	f = &fake{}
	r.LLM = f
	r.Opt.In = in2
	st, err = r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.count() != 1 || f.reqs[0].Text != "First line, changed." {
		t.Fatalf("変更された a1 だけ訳し直すはず: %+v", f.reqs)
	}
}
