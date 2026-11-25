// Package check は訳文の機械的な検証を行う。
// 問題があれば警告文字列を返し、翻訳器はそれを見て再試行や要確認リスト入りを判断する。
package check

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Torkkun/game-translate/internal/unit"
)

// ゲーム文字列でよく使われる、訳文にもそのまま残すべき記号列。
//
//	{0} {name}          … 書式指定 (C# / Unity)
//	%s %d %.1f          … 書式指定 (C 系)
//	<b> </color> <br/>  … リッチテキストのタグ
//	<1> </1>            … 数字だけのタグ
//	\n                  … エスケープされた改行
var placeholderRe = regexp.MustCompile(`\{[0-9A-Za-z_.:]*\}|%[-+#0]*\d*(?:\.\d+)?[sdifxXc]|</?[A-Za-z][^<>]*>|</?\d+>|%\d+|\\n`)

// Placeholders は text に含まれる記号列を出現順に返す。
func Placeholders(text string) []string {
	return placeholderRe.FindAllString(text, -1)
}

// Check は原文と訳文を比べて警告を返す。問題がなければ nil。
// keep は英語のまま残してよい語 (用語集で原語維持にした固有名詞など)。未翻訳の判定から除く。
func Check(source, target string, c unit.Constraints, keep ...string) []string {
	var w []string
	t := strings.TrimSpace(target)
	if t == "" {
		return []string{"訳文が空"}
	}
	if strings.Contains(t, "<|plamo") {
		w = append(w, "制御トークンが残っている")
	}
	if missing, extra := diff(Placeholders(source), Placeholders(target)); len(missing) > 0 || len(extra) > 0 {
		var parts []string
		if len(missing) > 0 {
			parts = append(parts, "不足 "+strings.Join(missing, " "))
		}
		if len(extra) > 0 {
			parts = append(parts, "余分 "+strings.Join(extra, " "))
		}
		w = append(w, "記号列の不一致: "+strings.Join(parts, " / "))
	}
	if ns, nt := strings.Count(source, "\n"), strings.Count(target, "\n"); ns != nt {
		w = append(w, fmt.Sprintf("改行数が違う (原文 %d, 訳文 %d)", ns, nt))
	}
	if c.MaxLen > 0 {
		if n := utf8.RuneCountInString(target); n > c.MaxLen {
			w = append(w, fmt.Sprintf("長すぎる (%d 文字 > 上限 %d)", n, c.MaxLen))
		}
	}
	if looksUntranslated(source, target, keep) {
		w = append(w, "未翻訳に見える")
	}
	return w
}

// looksUntranslated は訳文が原文のまま、または英字ばかりかを判定する。
func looksUntranslated(source, target string, keep []string) bool {
	// 長い語から消す ("Joe Dunn" を "Dunn" より先に)
	ks := append([]string(nil), keep...)
	sort.Slice(ks, func(i, j int) bool { return len(ks[i]) > len(ks[j]) })
	for _, k := range ks {
		if k != "" {
			source = strings.ReplaceAll(source, k, "")
			target = strings.ReplaceAll(target, k, "")
		}
	}
	s := strings.TrimSpace(source)
	t := strings.TrimSpace(target)
	if t == s && hasLetters(s) {
		return true
	}
	stripped := placeholderRe.ReplaceAllString(t, "")
	var latin, other int
	for _, r := range stripped {
		switch {
		case r < 128 && unicode.IsLetter(r):
			latin++
		case unicode.IsLetter(r):
			other++
		}
	}
	// 固有名詞を英字のまま残すことはあるので、英字が大半を占めるときだけ警告する
	return latin >= 4 && latin > other*2
}

func hasLetters(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// diff は多重集合として a にあって b にないもの、b にあって a にないものを返す。
func diff(a, b []string) (missing, extra []string) {
	count := map[string]int{}
	for _, x := range a {
		count[x]++
	}
	for _, x := range b {
		count[x]--
	}
	for k, v := range count {
		for ; v > 0; v-- {
			missing = append(missing, k)
		}
		for ; v < 0; v++ {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}
