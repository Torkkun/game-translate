// Package config はツール全体の設定 (gtl.yaml) を読む。
//
// 訳す言語・翻訳に使うモデル・フォントをここで切り替える。ファイルが無ければ既定値
// (英語 → 日本語、ローカルの PLaMo) を使う。コマンドのオプションはこの設定より優先する。
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultPath は設定ファイルの既定の場所 (リポジトリ直下)。環境変数 GTL_CONFIG で変えられる。
const DefaultPath = "gtl.yaml"

// Config はツール全体の設定。
type Config struct {
	SourceLang string `yaml:"source_lang"` // 原文の言語コード (例 en)
	TargetLang string `yaml:"target_lang"` // 訳文の言語コード (例 ja, ko, zh-Hans)
	LLM        LLM    `yaml:"llm"`
	Fonts      Fonts  `yaml:"fonts"`
	// Models は名前付きの翻訳エンジン (routes で使い分ける)。llm は "default" という名前で使える
	Models map[string]LLM `yaml:"models"`
	// Routes は文の種類 (翻訳単位の context.kind) → 使うエンジンの名前。
	// "default" は llm、"agent" は gtl translate では訳さず、Claude などのエージェントが gtl batch-export / batch-import で訳す。
	// "*" はほかのどれにも当たらない種類。書いていない種類は llm で訳す
	Routes map[string]string `yaml:"routes"`
}

// 特別なエンジンの名前
const (
	EngineDefault = "default" // llm の設定
	EngineAgent   = "agent"   // エージェント (Claude) が batch-export / batch-import で訳す
)

// Route は文の種類 (context.kind) に使うエンジンの名前を返す。
func (c *Config) Route(kind string) string {
	if r, ok := c.Routes[kind]; ok && r != "" {
		return r
	}
	if r, ok := c.Routes["*"]; ok && r != "" {
		return r
	}
	return EngineDefault
}

// Engine は名前からエンジンの設定を返す ("default" は llm)。
func (c *Config) Engine(name string) (LLM, bool) {
	if name == EngineDefault || name == "" {
		return c.LLM, true
	}
	l, ok := c.Models[name]
	return l, ok
}

// LLM は翻訳に使うモデルの設定。
type LLM struct {
	// Provider は plamo (llama.cpp 上の PLaMo 翻訳モデル) / openai (OpenAI 互換の
	// chat completions API: OpenAI, Ollama, LM Studio, llama.cpp の汎用モデルなど) /
	// anthropic (Claude API) のどれか。
	Provider string `yaml:"provider"`
	URL      string `yaml:"url"`   // API の URL。anthropic では空でよい
	Model    string `yaml:"model"` // モデル名
	// APIKeyEnv は API キーを入れた環境変数の名前。キーそのものは設定ファイルに書かない。
	APIKeyEnv string `yaml:"api_key_env"`
	// Effort は anthropic のときの思考の深さ (low / medium / high)。翻訳は low で足りることが多い。
	Effort string `yaml:"effort"`
	// Workers は同時に送る依頼の数。0 ならコマンドの既定値。
	Workers int `yaml:"workers"`
}

// Fonts は訳文を表示するフォントの設定。ゲームごとの組み込み方は各アダプターが決める。
type Fonts struct {
	// File はゲームに同梱するフォントファイル (TTF/OTF) のパス。
	File string `yaml:"file"`
	// Bitmap はドット絵風のゲーム (AGS など) に同梱するフォントファイル。
	Bitmap string `yaml:"bitmap"`
	// System は OS に入っているフォントを使うプラグイン向けのフォント名 (先にあるものから使う)。
	System []string `yaml:"system"`
}

// Default は設定ファイルが無いときの値。
func Default() *Config {
	c := &Config{}
	c.fill()
	return c
}

// Load は設定を読む。path が空なら GTL_CONFIG か gtl.yaml。ファイルが無ければ既定値。
func Load(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("GTL_CONFIG")
	}
	if path == "" {
		path = DefaultPath
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.fill()
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Refill は項目を書き換えたあとに、空の項目へ既定値を入れ直す。
func (c *Config) Refill() { c.fill() }

// fill は空の項目に既定値を入れる。
func (c *Config) fill() {
	if c.SourceLang == "" {
		c.SourceLang = "en"
	}
	if c.TargetLang == "" {
		c.TargetLang = "ja"
	}
	fillLLM(&c.LLM, true)
	for name, m := range c.Models {
		fillLLM(&m, false)
		c.Models[name] = m
	}
	if len(c.Fonts.System) == 0 {
		c.Fonts.System = DefaultSystemFonts(c.TargetLang)
	}
}

// fillLLM はエンジンの設定の空の項目に既定値を入れる。env なら GTL_LLM_URL で URL を上書きする (llm だけ)。
func fillLLM(l *LLM, env bool) {
	if l.Provider == "" {
		l.Provider = "plamo"
	}
	if v := os.Getenv("GTL_LLM_URL"); env && v != "" {
		l.URL = v
	}
	switch l.Provider {
	case "plamo":
		if l.URL == "" {
			l.URL = "http://127.0.0.1:8000"
		}
		if l.Model == "" {
			l.Model = "plamo-3-translate"
		}
	case "openai":
		if l.URL == "" {
			l.URL = "https://api.openai.com"
		}
		if l.APIKeyEnv == "" && strings.Contains(l.URL, "api.openai.com") {
			l.APIKeyEnv = "OPENAI_API_KEY"
		}
	case "anthropic":
		if l.Model == "" {
			l.Model = "claude-opus-5-5"
		}
		if l.APIKeyEnv == "" {
			l.APIKeyEnv = "ANTHROPIC_API_KEY"
		}
		if l.Effort == "" {
			l.Effort = "low"
		}
	}
}

// Validate は設定の誤りを返す。
func (c *Config) Validate() error {
	if err := validateLLM("llm", c.LLM); err != nil {
		return err
	}
	for name, m := range c.Models {
		if name == EngineDefault || name == EngineAgent {
			return fmt.Errorf("models に %q という名前は使えません (予約済み)", name)
		}
		if err := validateLLM("models."+name, m); err != nil {
			return err
		}
	}
	for kind, name := range c.Routes {
		if name == EngineDefault || name == EngineAgent {
			continue
		}
		if _, ok := c.Models[name]; !ok {
			return fmt.Errorf("routes.%s のエンジン %q が models にありません", kind, name)
		}
	}
	return nil
}

func validateLLM(where string, l LLM) error {
	switch l.Provider {
	case "plamo", "anthropic":
	case "openai":
		if l.Model == "" {
			return fmt.Errorf("%s.provider が openai のときは %s.model が必要です", where, where)
		}
	default:
		return fmt.Errorf("%s.provider は plamo / openai / anthropic のどれかです (%q)", where, l.Provider)
	}
	return nil
}

// APIKey は設定された環境変数から API キーを読む。
func (l LLM) APIKey() string {
	if l.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(l.APIKeyEnv)
}

// Suffix は言語ごとの出力ファイル名に付ける文字列 (例 ".ja")。
func (c *Config) Suffix() string { return "." + c.TargetLang }
