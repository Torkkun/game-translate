package main

import (
	"flag"
	"fmt"

	"github.com/Torkkun/game-translate/internal/config"
	"github.com/Torkkun/game-translate/internal/llm"
)

// llmFlags は翻訳モデルを使うコマンドに共通のオプション。空なら gtl.yaml の値を使う。
type llmFlags struct {
	config   *string
	provider *string
	url      *string
	model    *string
	source   *string
	target   *string
}

func addLLMFlags(fs *flag.FlagSet) *llmFlags {
	return &llmFlags{
		config:   fs.String("config", "", "全体設定ファイル (既定: 環境変数 GTL_CONFIG か gtl.yaml)"),
		provider: fs.String("provider", "", "翻訳モデルの種類: plamo / openai / anthropic (既定: gtl.yaml)"),
		url:      fs.String("llm", "", "API の URL (既定: gtl.yaml、環境変数 GTL_LLM_URL)"),
		model:    fs.String("model", "", "モデル名 (既定: gtl.yaml)"),
		source:   fs.String("source", "", "原文の言語コード (既定: gtl.yaml の source_lang)"),
		target:   fs.String("target", "", "訳文の言語コード (既定: gtl.yaml の target_lang)"),
	}
}

// load は設定ファイルを読み、オプションで上書きする。
func (f *llmFlags) load() (*config.Config, error) {
	c, err := config.Load(*f.config)
	if err != nil {
		return nil, err
	}
	if *f.provider != "" && *f.provider != c.LLM.Provider {
		// 種類を変えたら URL やモデルの既定値も入れ直す
		c.LLM = config.LLM{Provider: *f.provider, APIKeyEnv: c.LLM.APIKeyEnv, Workers: c.LLM.Workers}
		c.Refill()
	}
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&c.LLM.URL, *f.url)
	set(&c.LLM.Model, *f.model)
	set(&c.SourceLang, *f.source)
	set(&c.TargetLang, *f.target)
	if *f.target != "" {
		c.Fonts.System = config.DefaultSystemFonts(c.TargetLang)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.LLM.Provider == "anthropic" && c.LLM.APIKey() == "" {
		return nil, fmt.Errorf("環境変数 %s に API キーを入れてください (Claude API を使うため)", c.LLM.APIKeyEnv)
	}
	return c, nil
}

// backend は設定に合った翻訳器を作る。
func backend(c *config.Config) llm.Backend {
	return llm.NewBackend(llm.Settings{
		Provider: c.LLM.Provider,
		URL:      c.LLM.URL,
		Model:    c.LLM.Model,
		APIKey:   c.LLM.APIKey(),
		Effort:   c.LLM.Effort,
		Names:    config.LangName,
	})
}
