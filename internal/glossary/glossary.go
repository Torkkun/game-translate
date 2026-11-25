// Package glossary は人名・地名などの用語集を扱う。
// 原文に出てくる語だけをモデルに渡し、訳文で指定の訳語が使われたかを確認する。
package glossary

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/Torkkun/game-translate/internal/llm"
)

// Term は用語 1 件。
type Term struct {
	Source        string `yaml:"source"`
	Target        string `yaml:"target"`
	Note          string `yaml:"note,omitempty"`           // メモ (モデルには渡さない)
	CaseSensitive bool   `yaml:"case_sensitive,omitempty"` // 既定は大文字小文字を区別しない
	// Aliases はモデルが訳語の代わりに使いがちな表記 (例: Holmes に対する「ホームズ」)。
	// 訳文に訳語が無く別名がある場合、別名を訳語に置き換える。
	Aliases []string `yaml:"aliases,omitempty"`
}

// ApplyAliases は、訳文に訳語が無い用語について、別名を訳語に置き換える。
// 別名はカタカナ語の途中には一致させない (「ダン」が「ダンス」に当たらないように)。
func ApplyAliases(target string, terms []Term) string {
	for _, t := range terms {
		if strings.Contains(target, t.Target) {
			continue
		}
		// 長い別名から置き換える (「ジョン・ブラックサド」を「ブラックサド」より先に)
		aliases := append([]string(nil), t.Aliases...)
		sortByLenDesc(aliases)
		for _, a := range aliases {
			target = replaceWord(target, a, t.Target)
		}
	}
	return target
}

func sortByLenDesc(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && len(s[j]) > len(s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func isKatakana(r rune) bool { return (r >= 'ァ' && r <= 'ヴ') || r == 'ー' }

// replaceWord は old の前後がカタカナでない箇所だけを new に置き換える。
func replaceWord(s, old, new string) string {
	if old == "" {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, old)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		before, _ := utf8.DecodeLastRuneInString(s[:i])
		after, _ := utf8.DecodeRuneInString(s[i+len(old):])
		if (i > 0 && isKatakana(before)) || (i+len(old) < len(s) && isKatakana(after)) {
			b.WriteString(s[:i+len(old)])
		} else {
			b.WriteString(s[:i])
			b.WriteString(new)
		}
		s = s[i+len(old):]
	}
}

// Glossary は用語集。
type Glossary struct {
	Terms []Term `yaml:"terms"`
	res   []*regexp.Regexp
}

// Load は YAML の用語集を読む。path が空なら空の用語集を返す。
func Load(path string) (*Glossary, error) {
	g := &Glossary{}
	if path == "" {
		return g, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, g); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := g.compile(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return g, nil
}

// New はプログラムから用語集を作る。
func New(terms []Term) (*Glossary, error) {
	g := &Glossary{Terms: terms}
	return g, g.compile()
}

func (g *Glossary) compile() error {
	g.res = make([]*regexp.Regexp, len(g.Terms))
	for i, t := range g.Terms {
		if strings.TrimSpace(t.Source) == "" || strings.TrimSpace(t.Target) == "" {
			return fmt.Errorf("terms[%d]: source と target は必須です", i)
		}
		// 単語の途中には一致させない (例: "Gus" が "August" に当たらないように)
		p := `(?:^|[^\pL\pN])` + regexp.QuoteMeta(t.Source) + `(?:$|[^\pL\pN])`
		if !t.CaseSensitive {
			p = `(?i)` + p
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("terms[%d]: %w", i, err)
		}
		g.res[i] = re
	}
	return nil
}

// Match は text に出てくる用語を返す。
func (g *Glossary) Match(text string) []Term {
	var out []Term
	for i, re := range g.res {
		if re.MatchString(text) {
			out = append(out, g.Terms[i])
		}
	}
	return out
}

// Missing は訳文に訳語が見当たらない用語を返す。
func Missing(target string, terms []Term) []Term {
	var out []Term
	for _, t := range terms {
		if !strings.Contains(target, t.Target) {
			out = append(out, t)
		}
	}
	return out
}

// ToLLM はモデルに渡す形に変換する。
func ToLLM(terms []Term) []llm.Term {
	out := make([]llm.Term, len(terms))
	for i, t := range terms {
		out[i] = llm.Term{Source: t.Source, Target: t.Target}
	}
	return out
}
