package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func names(c string) string { return map[string]string{"en": "English", "ko": "Korean"}[c] }

func TestOpenAIRequest(t *testing.T) {
	c := NewOpenAI("http://x/v1/", "m", "", names)
	if c.BaseURL != "http://x" {
		t.Fatal(c.BaseURL)
	}
	b, err := c.BuildRequest(Request{Text: "Hi {0}", SourceLang: "en", TargetLang: "ko",
		Glossary: []Term{{Source: "Alice", Target: "Alice"}}, History: []Pair{{Source: "A", Target: "가"}}})
	if err != nil {
		t.Fatal(err)
	}
	var req openAIRequest
	json.Unmarshal(b, &req)
	if len(req.Messages) != 4 || req.Messages[3].Content != "Hi {0}" || req.Messages[2].Content != "가" || req.Temperature != nil {
		t.Fatalf("%s", b)
	}
	sys := req.Messages[0].Content
	if !strings.Contains(sys, "from English into Korean") || !strings.Contains(sys, "Alice => Alice") {
		t.Fatal(sys)
	}
}

func TestAnthropicParams(t *testing.T) {
	c := NewAnthropic("claude-opus-5-5", "k", "low", names)
	p := c.Params(Request{Text: "Hi", SourceLang: "en", TargetLang: "ko", History: []Pair{{Source: "A", Target: "가"}}})
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"fallbacks":"default"`, `"effort":"low"`, `"role":"assistant"`, `into Korean`} {
		if !strings.Contains(s, want) {
			t.Fatalf("%s が無い: %s", want, s)
		}
	}
	if strings.Contains(s, "temperature") {
		t.Fatal("temperature は送らない")
	}
}
