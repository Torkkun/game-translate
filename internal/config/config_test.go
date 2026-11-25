package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingUsesDefaults(t *testing.T) {
	t.Setenv("GTL_LLM_URL", "")
	c, err := Load(filepath.Join(t.TempDir(), "none.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.TargetLang != "ja" || c.LLM.Provider != "plamo" || c.LLM.URL != "http://127.0.0.1:8000" || c.Fonts.System[0] != "Yu Mincho" {
		t.Fatalf("%+v", c)
	}
}

func TestLoadKoreanAnthropic(t *testing.T) {
	t.Setenv("GTL_LLM_URL", "")
	p := filepath.Join(t.TempDir(), "gtl.yaml")
	os.WriteFile(p, []byte("target_lang: ko\nllm:\n  provider: anthropic\n"), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.LLM.Model != "claude-opus-5-5" || c.LLM.APIKeyEnv != "ANTHROPIC_API_KEY" || c.Fonts.System[0] != "Batang" || c.Suffix() != ".ko" {
		t.Fatalf("%+v", c)
	}
}

func TestRejectUnknownProvider(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gtl.yaml")
	os.WriteFile(p, []byte("llm:\n  provider: foo\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("知らない provider はエラーのはず")
	}
}
