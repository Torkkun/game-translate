package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranslateRequestAndResponse(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Errorf("リクエストが JSON でない: %v", err)
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":" 二つ目だ。<|plamo:tag|>"}}]}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "plamo-3-translate")
	out, err := c.Translate(context.Background(), Request{
		Text:       "The second one.",
		SourceLang: "en",
		TargetLang: "ja",
		Context:    "Speaker: Gus",
		Glossary:   []Term{{Source: "Gus", Target: "ガス"}},
		History:    []Pair{{Source: "The first one.", Target: "一つ目だ。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "二つ目だ。" {
		t.Fatalf("制御トークンと空白が除かれていない: %q", out)
	}

	if len(got.Messages) != 3 || got.Messages[0].Role != "user" || got.Messages[1].Role != "assistant" || got.Messages[2].Content != "The second one." {
		t.Fatalf("履歴の並びが違う: %+v", got.Messages)
	}
	kw := got.ChatTemplateKwargs
	if kw["target_lang"] != "ja" || kw["source_lang"] != "en" || kw["context"] != "Speaker: Gus" {
		t.Fatalf("chat_template_kwargs が違う: %+v", kw)
	}
	if _, ok := kw["form"]; ok {
		t.Fatalf("空の form は送らない: %+v", kw)
	}
	if g, ok := kw["glossary"].([]any); !ok || len(g) != 1 {
		t.Fatalf("glossary が違う: %+v", kw["glossary"])
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "loading model", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "m").Translate(context.Background(), Request{Text: "x"}); err == nil {
		t.Fatal("503 はエラーになるべき")
	}
}
