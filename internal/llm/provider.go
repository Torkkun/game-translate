package llm

import "context"

// Backend は翻訳器と、接続確認。
type Backend interface {
	Translator
	Health(ctx context.Context) error
}

// Settings は Backend を作るための設定 (config.LLM から作る)。
type Settings struct {
	Provider string // plamo / openai / anthropic
	URL      string
	Model    string
	APIKey   string
	Effort   string
	Names    LangNamer
}

// NewBackend は設定に合った翻訳器を作る。
func NewBackend(s Settings) Backend {
	switch s.Provider {
	case "openai":
		return NewOpenAI(s.URL, s.Model, s.APIKey, s.Names)
	case "anthropic":
		return NewAnthropic(s.Model, s.APIKey, s.Effort, s.Names)
	default:
		return New(s.URL, s.Model)
	}
}
