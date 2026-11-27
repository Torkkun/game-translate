package translate

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/unit"
)

// 訳の不統一と、用語集に無い固有名詞の候補を探す (gtl consistency)。
//
// 1 行ずつの機械翻訳では、同じ UI の語や人名が行ごとに違う訳になりやすい
// (例: 同じ「Defend」が「防衛」と「防御」、人名の Hope が「ホープ」と「希望」)。
// どちらも訳した後でまとめて見ないと気付きにくいので、一覧にして fixes.tsv と用語集で直す。

// Variant は同じ原文に付いた訳の 1 通り。
type Variant struct {
	Target string
	IDs    []string
}

// InconsistentGroup は同じ原文なのに訳が複数ある組。
type InconsistentGroup struct {
	Source   string
	Variants []Variant // 多い順
}

// Count はこの組の行数。
func (g InconsistentGroup) Count() int {
	n := 0
	for _, v := range g.Variants {
		n += len(v.IDs)
	}
	return n
}

var spaceRe = regexp.MustCompile(`\s+`)

// normSource は原文を比べる形にする (前後の空白・末尾の句読点・大文字小文字の違いを無視する)。
func normSource(s string) string {
	s = strings.ToLower(spaceRe.ReplaceAllString(strings.TrimSpace(s), " "))
	return strings.TrimRight(s, ".!?:;…, ")
}

// normTarget は訳文を比べる形にする (末尾の句読点・前後の空白の違いは不統一としない)。
func normTarget(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimRight(s, "。．！？!?：:…、 ")
}

// Inconsistent は、同じ原文 (maxWords 語以下。0 なら長さを問わない) に違う訳が付いている組を返す。行数の多い順。
func Inconsistent(units []unit.Unit, maxWords int) []InconsistentGroup {
	type acc struct {
		source string
		order  []string
		ids    map[string][]string
	}
	groups := map[string]*acc{}
	var keys []string
	for _, u := range units {
		if strings.TrimSpace(u.Target) == "" || strings.TrimSpace(u.Source) == "" {
			continue
		}
		if maxWords > 0 && len(strings.Fields(u.Source)) > maxWords {
			continue
		}
		k := normSource(u.Source)
		if k == "" {
			continue
		}
		a := groups[k]
		if a == nil {
			a = &acc{source: strings.TrimSpace(u.Source), ids: map[string][]string{}}
			groups[k] = a
			keys = append(keys, k)
		}
		t := normTarget(u.Target)
		if _, ok := a.ids[t]; !ok {
			a.order = append(a.order, t)
		}
		a.ids[t] = append(a.ids[t], u.ID)
	}
	var out []InconsistentGroup
	for _, k := range keys {
		a := groups[k]
		if len(a.order) < 2 {
			continue
		}
		g := InconsistentGroup{Source: a.source}
		for _, t := range a.order {
			g.Variants = append(g.Variants, Variant{Target: t, IDs: a.ids[t]})
		}
		sort.SliceStable(g.Variants, func(i, j int) bool { return len(g.Variants[i].IDs) > len(g.Variants[j].IDs) })
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count() > out[j].Count() })
	return out
}

// NameCandidate は用語集に無い固有名詞らしい語。
type NameCandidate struct {
	Word  string
	Count int    // 文の途中に大文字で出てきた回数
	Lower int    // 小文字で出てきた回数 (多ければ普通の語)
	ID    string // 出てくる行の例
}

// 大文字で始まる語の並び (Red Hat、Hope など)。
var capWordsRe = regexp.MustCompile(`\b[A-Z][a-z]+(?:[ '-][A-Z][a-z]+)*\b`)

// 固有名詞の候補にしない語 (文の途中でも大文字で書かれやすい語)。
var notNames = map[string]bool{
	"I": true, "Mr": true, "Mrs": true, "Ms": true, "Dr": true, "Oh": true, "OK": true, "Okay": true,
	"Monday": true, "Tuesday": true, "Wednesday": true, "Thursday": true, "Friday": true, "Saturday": true, "Sunday": true,
	"January": true, "February": true, "March": true, "April": true, "June": true, "July": true,
	"August": true, "September": true, "October": true, "November": true, "December": true,
}

// NameCandidates は、文の途中に大文字で min 回以上出てくるのに用語集に無い語を返す。
// 人名・地名・団体名・艦名は、用語集で訳 (または英語のまま残すこと) を決めてから訳さないと、
// 行ごとに訳がぶれたり、普通の語として訳されたりする (Hope → 希望)。
func NameCandidates(units []unit.Unit, g *glossary.Glossary, min int) []NameCandidate {
	type acc struct {
		count int
		id    string
	}
	caps := map[string]*acc{}
	var order []string
	text := make([]string, 0, len(units))
	for _, u := range units {
		text = append(text, u.Source)
		for _, loc := range capWordsRe.FindAllStringIndex(u.Source, -1) {
			w := u.Source[loc[0]:loc[1]]
			if sentenceStart(u.Source[:loc[0]]) {
				// 文頭の語は大文字で当然なので外す。続く語があれば残りを候補にする (Ask Hope → Hope)
				_, rest, ok := strings.Cut(w, " ")
				if !ok {
					continue
				}
				w = rest
			}
			if notNames[w] {
				continue
			}
			a := caps[w]
			if a == nil {
				a = &acc{id: u.ID}
				caps[w] = a
				order = append(order, w)
			}
			a.count++
		}
	}
	all := strings.Join(text, "\n")
	var out []NameCandidate
	for _, w := range order {
		a := caps[w]
		if a.count < min {
			continue
		}
		if g != nil && len(g.Match(w)) > 0 {
			continue
		}
		lower := countWord(all, strings.ToLower(w))
		if lower*2 >= a.count {
			continue // 普通の語 (文頭や見出しで大文字になっただけ)
		}
		out = append(out, NameCandidate{Word: w, Count: a.count, Lower: lower, ID: a.id})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// sentenceStart は before の後ろが文の始まりか (行頭・文末記号の後・引用符の直後)。
func sentenceStart(before string) bool {
	b := strings.TrimRightFunc(before, unicode.IsSpace)
	b = strings.TrimRight(b, `"'“‘([`)
	if b == "" {
		return true
	}
	switch b[len(b)-1] {
	case '.', '!', '?', ':', '\n', '>', ']':
		return true
	}
	return strings.HasSuffix(b, "…") || strings.HasSuffix(b, "\\n")
}

// countWord は text の中で w が単語として出てくる回数 (大文字小文字を区別する)。
func countWord(text, w string) int {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(w) + `\b`)
	return len(re.FindAllStringIndex(text, -1))
}
