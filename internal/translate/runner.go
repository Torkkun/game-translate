// Package translate は TranslationUnit の JSONL を一括翻訳する。
//
//   - 同じシーンの行は順番に訳し、直前の数行 (原文と訳文) を履歴としてモデルに渡す
//   - 別のシーン同士は並列に訳す (llama.cpp の --parallel スロットを使う)
//   - 翻訳メモリにある原文はモデルを呼ばずに再利用する
//   - 1 件訳すごとに出力ファイルへ追記するので、中断しても --resume 相当で続きから再開できる
//   - 検証で警告が出たら温度を上げて 1 回だけ再試行し、警告の少ない方を採用する
package translate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Torkkun/game-translate/internal/check"
	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/llm"
	"github.com/Torkkun/game-translate/internal/tm"
	"github.com/Torkkun/game-translate/internal/unit"
)

// Options は翻訳の設定。
type Options struct {
	In, Out    string
	SourceLang string // "en"
	TargetLang string // "ja"
	Form       string // "plain" / "polite" / ""
	Model      string // 翻訳メモリに記録するモデル名
	Workers    int    // 同時に訳すシーン数
	History    int    // 履歴として渡す直前の行数
	Limit      int    // 0 より大きければ、今回訳す件数の上限 (試験翻訳用)
	UseTM      bool   // 翻訳メモリの訳を再利用するか
	Retries    int    // 通信エラー時の再試行回数
	// Speakers は話者名 → プロフィール (性別・口調など)。文脈としてモデルに渡す
	Speakers map[string]Speaker
	// Attempts は警告が出たときに試す回数の上限 (最初の 1 回を含む, 既定 2)
	Attempts int
	// RetryWarned が true なら、前回の結果で警告が残った機械翻訳の行を訳し直す (翻訳メモリは使わない)
	RetryWarned bool
}

// Stats は実行結果の集計。
type Stats struct {
	Total      int // 入力件数
	Skipped    int // 訳済みのため飛ばした件数
	Translated int // モデルで訳した件数
	FromTM     int // 翻訳メモリから再利用した件数
	Warned     int // 警告が残った件数
	Failed     int // エラーで訳せなかった件数
	Deferred   int // エージェント (Claude) に回した件数 (routes が agent。batch-export で渡す)
	Elapsed    time.Duration
}

// Runner は一括翻訳を実行する。
type Runner struct {
	LLM llm.Translator
	// Route は翻訳単位ごとに使う翻訳器を選ぶ (文の種類で PLaMo と Claude を使い分けるなど)。nil なら LLM。
	// agent が true の単位は訳さずに残す (エージェントが gtl batch-export / batch-import で訳す)
	Route    func(u unit.Unit) (tr llm.Translator, agent bool)
	TM       *tm.TM // nil なら翻訳メモリを使わない
	Glossary *glossary.Glossary
	Opt      Options
	Logf     func(format string, args ...any)
}

func (r *Runner) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

// Run は翻訳を実行し、最後に Out を入力順に並べ直して書き出す。
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	start := time.Now()
	var st Stats
	if r.Glossary == nil {
		r.Glossary = &glossary.Glossary{}
	}
	if r.Opt.Workers <= 0 {
		r.Opt.Workers = 1
	}

	units, err := unit.ReadJSONL(r.Opt.In)
	if err != nil {
		return st, err
	}
	st.Total = len(units)
	for i := range units {
		units[i].Hash = unit.SourceHash(units[i].Source)
		if units[i].Status == "" {
			units[i].Status = unit.StatusNew
		}
	}

	// 前回までの結果 (追記ファイルなので同じ ID は後の行が優先)
	prev, err := loadPrevious(r.Opt.Out)
	if err != nil {
		return st, err
	}

	// 訳済みなら結果を引き継ぐ。原文が変わっていたら (hash 不一致) 訳し直す
	results := make([]unit.Unit, len(units))
	todo := make([]bool, len(units))
	nTodo := 0
	for i, u := range units {
		p, ok := prev[u.ID]
		retry := r.Opt.RetryWarned && ok && p.Status == unit.StatusMachine && len(p.Warnings) > 0
		if ok && !retry && p.Hash == u.Hash && p.Status.Done() && p.Target != "" {
			results[i] = p
			st.Skipped++
			continue
		}
		if u.Status.Done() && u.Target != "" { // 入力側ですでに訳がある
			results[i] = u
			st.Skipped++
			continue
		}
		results[i] = u
		if r.Route != nil {
			if _, agent := r.Route(u); agent {
				st.Deferred++
				continue
			}
		}
		if r.Opt.Limit > 0 && nTodo >= r.Opt.Limit {
			continue
		}
		todo[i] = true
		nTodo++
	}
	r.logf("入力 %d 件 / 訳済み %d 件 / 今回訳す %d 件 / エージェントに回す %d 件", st.Total, st.Skipped, nTodo, st.Deferred)

	app, err := unit.OpenAppender(r.Opt.Out)
	if err != nil {
		return st, err
	}

	// シーンごとにまとめる。シーン無しの行は 1 行ずつ独立に扱う
	groups := groupByScene(units)

	var (
		done                               atomic.Int64
		translated, fromTM, warned, failed atomic.Int64
		firstErr                           error
		errOnce                            sync.Once
	)
	jobs := make(chan []int)
	var wg sync.WaitGroup
	for w := 0; w < r.Opt.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idxs := range jobs {
				var history []llm.Pair
				for _, i := range idxs {
					if ctx.Err() != nil {
						return
					}
					if !todo[i] {
						if t := results[i].Target; t != "" {
							history = appendHistory(history, llm.Pair{Source: results[i].Source, Target: t}, r.Opt.History)
						}
						continue
					}
					t0 := time.Now()
					u, src, err := r.translateOne(ctx, units[i], history)
					n := done.Add(1)
					if err != nil {
						failed.Add(1)
						r.logf("[%d/%d] %s 失敗: %v", n, nTodo, units[i].ID, err)
						if ctx.Err() != nil {
							errOnce.Do(func() { firstErr = ctx.Err() })
						}
						continue
					}
					results[i] = u
					if err := app.Append(u); err != nil {
						errOnce.Do(func() { firstErr = err })
					}
					switch src {
					case "tm":
						fromTM.Add(1)
					default:
						translated.Add(1)
					}
					mark := ""
					if len(u.Warnings) > 0 {
						warned.Add(1)
						mark = " ⚠ " + strings.Join(u.Warnings, "; ")
					}
					r.logf("[%d/%d] %s (%s, %.1fs)%s", n, nTodo, u.ID, src, time.Since(t0).Seconds(), mark)
					history = appendHistory(history, llm.Pair{Source: u.Source, Target: u.Target}, r.Opt.History)
				}
			}
		}()
	}

	for _, g := range groups {
		if !anyTodo(g, todo) {
			continue
		}
		select {
		case jobs <- g:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	_ = app.Close() // 追記用のファイル。直後に全体を書き直すので、閉じるときの失敗は問題にならない

	st.Translated = int(translated.Load())
	st.FromTM = int(fromTM.Load())
	st.Warned = int(warned.Load())
	st.Failed = int(failed.Load())

	// 追記ファイルを入力順の完全な一覧に書き直す
	if err := unit.WriteJSONL(r.Opt.Out, results); err != nil {
		return st, err
	}
	st.Elapsed = time.Since(start)
	if firstErr != nil {
		return st, firstErr
	}
	return st, nil
}

// translateOne は 1 件訳す。src は "tm" か "llm"。
func (r *Runner) translateOne(ctx context.Context, u unit.Unit, history []llm.Pair) (unit.Unit, string, error) {
	key := tm.Key(u.Source, u.Context.Speaker, r.Opt.TargetLang, r.Opt.Form)
	terms := r.Glossary.Match(u.Source)
	tr := r.LLM
	if r.Route != nil {
		if t, _ := r.Route(u); t != nil {
			tr = t
		}
	}

	if r.TM != nil && r.Opt.UseTM {
		if t, ok, err := r.TM.Get(key); err != nil {
			return u, "", err
		} else if ok {
			u.Target = t
			u.Status = unit.StatusMachine
			u.Warnings = r.validate(u, terms)
			return u, "tm", nil
		}
	}

	req := llm.Request{
		Text:       u.Source,
		SourceLang: r.Opt.SourceLang,
		TargetLang: r.Opt.TargetLang,
		Form:       r.Opt.Form,
		Context:    buildContext(u, r.Opt.Speakers),
		Glossary:   glossary.ToLLM(terms),
		History:    history,
	}
	out, err := r.call(ctx, tr, req)
	if err != nil {
		return u, "", err
	}
	u.Target = glossary.ApplyAliases(out, terms)
	u.Warnings = r.validate(u, terms)

	// 警告が出たら温度を少しずつ上げてやり直し、警告の少ない訳を採る
	attempts := r.Opt.Attempts
	if attempts < 2 {
		attempts = 2
	}
	temps := []float64{0.4, 0.7, 0.9, 1.0}
	for a := 1; a < attempts && len(u.Warnings) > 0; a++ {
		req.Temperature = temps[min(a-1, len(temps)-1)]
		if out2, err := r.call(ctx, tr, req); err == nil {
			u2 := u
			u2.Target = glossary.ApplyAliases(out2, terms)
			u2.Warnings = r.validate(u2, terms)
			if len(u2.Warnings) < len(u.Warnings) {
				u = u2
			}
		}
	}

	u.Status = unit.StatusMachine
	if r.TM != nil && len(u.Warnings) == 0 {
		if err := r.TM.Put(key, u.Source, u.Target, r.Opt.TargetLang, r.Opt.Model); err != nil {
			return u, "", err
		}
	}
	return u, "llm", nil
}

func (r *Runner) call(ctx context.Context, tr llm.Translator, req llm.Request) (string, error) {
	var err error
	for attempt := 0; attempt <= r.Opt.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		var out string
		out, err = tr.Translate(ctx, req)
		if err == nil {
			return out, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", err
}

func (r *Runner) validate(u unit.Unit, terms []glossary.Term) []string {
	var keep []string
	for _, t := range terms {
		if t.Source == t.Target { // 原語のまま残す用語
			keep = append(keep, t.Target)
		}
	}
	w := check.Check(u.Source, u.Target, u.Constraints, keep...)
	for _, t := range glossary.Missing(u.Target, terms) {
		w = append(w, fmt.Sprintf("用語集の訳語が無い: %s → %s", t.Source, t.Target))
	}
	if sp, ok := r.Opt.Speakers[u.Context.Speaker]; ok {
		w = append(w, checkSpeaker(u.Target, sp)...)
	}
	return w
}

// unknownSpeakerNote は、話者が分からない台詞に添える指示。
const unknownSpeakerNote = "話者は不明。原文から話者の性別や性格がはっきり分からないときは、" +
	"「俺」「僕」「あたし」や「〜だぜ」「〜わよ」のように性別が強く出る言い方を避け、中立的な口調で訳すこと。"

// buildContext はモデルに渡す状況説明を作る。
func buildContext(u unit.Unit, speakers map[string]Speaker) string {
	var parts []string
	if u.Context.Kind == "ui" {
		parts = append(parts, "This is a user interface string in a video game.")
	}
	if u.Context.Note != "" {
		parts = append(parts, u.Context.Note)
	}
	if s := u.Context.Speaker; s != "" {
		if sp, ok := speakers[s]; ok {
			name := s
			if sp.Name != "" && sp.Name != s {
				name = sp.Name
			}
			if d := sp.Describe(); d != "" {
				parts = append(parts, "Speaker: "+name+"\n話者について: "+d+"\nこの話者の口調 (一人称・語尾) に合わせて訳すこと。")
			} else {
				parts = append(parts, "Speaker: "+name)
			}
		} else {
			parts = append(parts, "Speaker: "+s)
		}
	} else if speakers != nil && u.Context.Kind != "ui" {
		// キャラクター表があるのに話者が分からない台詞: 性別が強く出る言い方を避ける
		parts = append(parts, unknownSpeakerNote)
	}
	return strings.Join(parts, "\n")
}

func appendHistory(h []llm.Pair, p llm.Pair, max int) []llm.Pair {
	if max <= 0 {
		return nil
	}
	h = append(h, p)
	if len(h) > max {
		h = h[len(h)-max:]
	}
	return h
}

func groupByScene(units []unit.Unit) [][]int {
	var groups [][]int
	index := map[string]int{}
	for i, u := range units {
		s := u.Context.Scene
		if s == "" {
			groups = append(groups, []int{i})
			continue
		}
		g, ok := index[s]
		if !ok {
			g = len(groups)
			index[s] = g
			groups = append(groups, nil)
		}
		groups[g] = append(groups[g], i)
	}
	return groups
}

func anyTodo(idxs []int, todo []bool) bool {
	for _, i := range idxs {
		if todo[i] {
			return true
		}
	}
	return false
}

func loadPrevious(path string) (map[string]unit.Unit, error) {
	units, err := unit.ReadJSONL(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]unit.Unit{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := make(map[string]unit.Unit, len(units))
	for _, u := range units {
		m[u.ID] = u
	}
	return m, nil
}

// Recheck はモデルを呼ばずに、別名の置き換えと検証だけをやり直す (用語集や話者プロフィールを直した後用)。
// 人が確認した行 (reviewed / locked) には触らない。
func (r *Runner) Recheck(u unit.Unit) unit.Unit {
	if u.Status != unit.StatusMachine || u.Target == "" {
		return u
	}
	if r.Glossary == nil {
		r.Glossary = &glossary.Glossary{}
	}
	terms := r.Glossary.Match(u.Source)
	u.Target = glossary.ApplyAliases(u.Target, terms)
	u.Warnings = r.validate(u, terms)
	return u
}
