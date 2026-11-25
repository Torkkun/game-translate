package config

import "strings"

// langNames は言語コードから英語の言語名 (モデルへの指示に使う) と、その言語での名前
// (ゲームの言語メニューに出す名前) を引く。
var langNames = map[string][2]string{
	"ja":      {"Japanese", "日本語"},
	"ko":      {"Korean", "한국어"},
	"zh-Hans": {"Simplified Chinese", "简体中文"},
	"zh-Hant": {"Traditional Chinese", "繁體中文"},
	"en":      {"English", "English"},
	"fr":      {"French", "Français"},
	"de":      {"German", "Deutsch"},
	"es":      {"Spanish", "Español"},
	"it":      {"Italian", "Italiano"},
	"pt-BR":   {"Brazilian Portuguese", "Português (Brasil)"},
	"ru":      {"Russian", "Русский"},
}

// LangName はモデルへの指示に使う英語の言語名。知らない言語はコードをそのまま返す。
func LangName(code string) string {
	if n, ok := langNames[code]; ok {
		return n[0]
	}
	return code
}

// NativeName はその言語自身での言語名 (言語メニューの表示用)。
func NativeName(code string) string {
	if n, ok := langNames[code]; ok {
		return n[1]
	}
	return code
}

// IsCJK は漢字・かな・ハングルを使う言語か (フォントの差し替えや行の長さの目安に使う)。
func IsCJK(code string) bool {
	return code == "ja" || code == "ko" || strings.HasPrefix(code, "zh")
}

// DefaultSystemFonts は Windows に標準で入っているその言語向けのフォント名。
func DefaultSystemFonts(code string) []string {
	switch {
	case code == "ja":
		return []string{"Yu Mincho", "MS PMincho", "Meiryo"}
	case code == "ko":
		return []string{"Batang", "Malgun Gothic"}
	case code == "zh-Hant":
		return []string{"PMingLiU", "Microsoft JhengHei"}
	case strings.HasPrefix(code, "zh"):
		return []string{"SimSun", "Microsoft YaHei"}
	}
	return nil
}
