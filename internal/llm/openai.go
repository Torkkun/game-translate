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

// OpenAI は OpenAI 互換の chat completions API (OpenAI, Ollama, LM Studio,
// llama.cpp server で動かす汎用モデルなど) を使う翻訳器。
type OpenAI struct {
	BaseURL string // 例 https://api.openai.com, http://127.0.0.1:11434
	Model   string
	APIKey  string // 空なら Authorization を送らない (ローカルのサーバー向け)
	Names   LangNamer
	HTTP    *http.Client
}

// NewOpenAI は OpenAI 互換 API の翻訳器を作る。
func NewOpenAI(baseURL, model, apiKey string, names LangNamer) *OpenAI {
	return &OpenAI{
		BaseURL: strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1"),
		Model:   model,
		APIKey:  apiKey,
		Names:   names,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

type openAIRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
}

// BuildRequest は API に送る JSON 本体を組み立てる (テスト用に公開)。
func (c *OpenAI) BuildRequest(r Request) ([]byte, error) {
	msgs := []message{{Role: "system", Content: SystemPrompt(r, c.Names)}}
	for _, h := range r.History {
		msgs = append(msgs, message{Role: "user", Content: h.Source}, message{Role: "assistant", Content: h.Target})
	}
	msgs = append(msgs, message{Role: "user", Content: r.Text})
	req := openAIRequest{Model: c.Model, Messages: msgs}
	if r.Temperature > 0 {
		t := r.Temperature
		req.Temperature = &t
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
func (c *OpenAI) Translate(ctx context.Context, r Request) (string, error) {
	body, err := c.BuildRequest(r)
	if err != nil {
		return "", err
	}
	raw, err := c.do(ctx, http.MethodPost, "/v1/chat/completions", body)
	if err != nil {
		return "", err
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("llm: 応答を解釈できません: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llm: choices が空です")
	}
	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}

// Health はモデル一覧を取れるか確認する。
func (c *OpenAI) Health(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/v1/models", nil)
	return err
}

func (c *OpenAI) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	return raw, nil
}
