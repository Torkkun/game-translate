// Package llm は翻訳に使うモデルを呼ぶ。Client (このファイル) は llama.cpp server 上の PLaMo 翻訳モデル用で、
// 汎用の LLM には OpenAI (openai.go) と Anthropic (anthropic.go) を使う。NewBackend で設定から選ぶ。
//
// PLaMo 3 Translate の GGUF はチャットテンプレートを内蔵しており、翻訳オプションは
// chat_template_kwargs (target_lang, source_lang, form, context, glossary) で渡す。
// 以前の訳は user/assistant の組として messages に並べると、訳の一貫性が上がる。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Term は用語集の 1 項目。
type Term struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Pair は翻訳履歴 1 件 (原文と訳文)。
type Pair struct {
	Source string
	Target string
}

// Request は 1 文の翻訳依頼。
type Request struct {
	Text        string
	SourceLang  string // 例 "en"。空ならモデルが判定
	TargetLang  string // 例 "ja"
	Form        string // "plain" / "polite" / 空 (日本語出力のときのみ有効)
	Context     string // 状況説明など
	Glossary    []Term
	History     []Pair // 直前の翻訳 (古い順)
	Temperature float64
	MaxTokens   int
}

// Translator は翻訳器のインターフェース (テストで差し替える)。
type Translator interface {
	Translate(ctx context.Context, r Request) (string, error)
}

// Client は llama.cpp server のクライアント。
type Client struct {
	BaseURL string // 例 http://127.0.0.1:8000
	Model   string // --alias で付けた名前
	HTTP    *http.Client
}

// New は Client を作る。
func New(baseURL, model string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model              string         `json:"model"`
	Messages           []message      `json:"messages"`
	Temperature        float64        `json:"temperature"`
	MaxTokens          int            `json:"max_tokens,omitempty"`
	Stop               []string       `json:"stop"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

// 訳文の末尾に残ることがあるモデルの制御トークン。
var stopTokens = []string{"<|plamo:tag|>", "<|plamo:bos|>"}

// BuildRequest は API に送る JSON 本体を組み立てる (テスト用に公開)。
func (c *Client) BuildRequest(r Request) ([]byte, error) {
	msgs := make([]message, 0, len(r.History)*2+1)
	for _, h := range r.History {
		msgs = append(msgs,
			message{Role: "user", Content: h.Source},
			message{Role: "assistant", Content: h.Target})
	}
	msgs = append(msgs, message{Role: "user", Content: r.Text})

	kw := map[string]any{}
	if r.TargetLang != "" {
		kw["target_lang"] = r.TargetLang
	}
	if r.SourceLang != "" {
		kw["source_lang"] = r.SourceLang
	}
	if r.Form != "" {
		kw["form"] = r.Form
	}
	if r.Context != "" {
		kw["context"] = r.Context
	}
	if len(r.Glossary) > 0 {
		kw["glossary"] = r.Glossary
	}

	maxTokens := r.MaxTokens
	if maxTokens == 0 {
		// 日本語は英語よりトークンが増えがちなので、原文の長さに余裕を持たせる
		maxTokens = 64 + len(r.Text)*2
	}
	req := chatRequest{
		Model:              c.Model,
		Messages:           msgs,
		Temperature:        r.Temperature,
		MaxTokens:          maxTokens,
		Stop:               stopTokens,
		ChatTemplateKwargs: kw,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(req); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Translate は 1 文を翻訳する。
func (c *Client) Translate(ctx context.Context, r Request) (string, error) {
	body, err := c.BuildRequest(r)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("llm: 応答を解釈できません: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llm: choices が空です")
	}
	return CleanOutput(cr.Choices[0].Message.Content), nil
}

// CleanOutput は訳文の前後の空白と、残った制御トークンを取り除く。
func CleanOutput(s string) string {
	for _, t := range stopTokens {
		if i := strings.Index(s, t); i >= 0 {
			s = s[:i]
		}
	}
	return strings.TrimSpace(s)
}

// Health はサーバーが応答できる状態か確認する。
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("llm: health HTTP %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
