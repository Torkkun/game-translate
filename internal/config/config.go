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
	l := &c.LLM
	if l.Provider == "" {
		l.Provider = "plamo"
	}
	if v := os.Getenv("GTL_LLM_URL"); v != "" {
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
	if len(c.Fonts.System) == 0 {
		c.Fonts.System = DefaultSystemFonts(c.TargetLang)
	}
}

// Validate は設定の誤りを返す。
func (c *Config) Validate() error {
	switch c.LLM.Provider {
	case "plamo", "anthropic":
	case "openai":
		if c.LLM.Model == "" {
			return fmt.Errorf("llm.provider が openai のときは llm.model が必要です")
		}
	default:
		return fmt.Errorf("llm.provider は plamo / openai / anthropic のどれかです (%q)", c.LLM.Provider)
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
