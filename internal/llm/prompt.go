package llm

import (
	"fmt"
	"strings"
)

// 汎用の LLM (openai / anthropic) に渡す指示。PLaMo はチャットテンプレートが翻訳用に
// 作られているので使わない。

// LangNamer は言語コードから英語の言語名を引く (config.LangName を渡す)。
type LangNamer func(code string) string

// SystemPrompt は翻訳の指示 (system) を組み立てる。
func SystemPrompt(r Request, name LangNamer) string {
	if name == nil {
		name = func(s string) string { return s }
	}
	var b strings.Builder
	src := "the source language"
	if r.SourceLang != "" {
		src = name(r.SourceLang)
	}
	tgt := name(r.TargetLang)
	fmt.Fprintf(&b, "You are a professional video game localizer. Translate each user message from %s into %s.\n", src, tgt)
	b.WriteString("Earlier user/assistant turns are previous lines of the same scene and their translations; keep names, tone and terminology consistent with them.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Reply with the translation only: no explanations, notes, romanization or surrounding quotes.\n")
	b.WriteString("- Keep placeholders, markup and escape sequences exactly as written, e.g. {0}, {name}, %s, %d, <b>, </color>, [p], \\n.\n")
	b.WriteString("- Keep the same number of line breaks as the source.\n")
	b.WriteString("- Translate naturally for players; keep the register and personality of the speaker.\n")
	switch r.Form {
	case "plain":
		b.WriteString("- Use the plain form (だ・である調) in narration and neutral text.\n")
	case "polite":
		b.WriteString("- Use the polite form (です・ます調).\n")
	}
	if len(r.Glossary) > 0 {
		b.WriteString("\nGlossary (always use these translations; a target equal to the source means keep it untranslated):\n")
		for _, t := range r.Glossary {
			fmt.Fprintf(&b, "- %s => %s\n", t.Source, t.Target)
		}
	}
	if r.Context != "" {
		b.WriteString("\nContext for this line:\n")
		b.WriteString(r.Context)
		b.WriteString("\n")
	}
	return b.String()
}
