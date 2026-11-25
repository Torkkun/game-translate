package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// Anthropic は Claude API を使う翻訳器。
//
// 拒否 (stop_reason=refusal) されたときはサーバー側のフォールバックで別のモデルが答える
// (server-side-fallback ベータ)。それでも訳が無ければエラーにして、呼び出し側の再試行に任せる。
type Anthropic struct {
	Client anthropic.Client
	Model  string
	Effort string // low / medium / high
	Names  LangNamer
}

// NewAnthropic は Claude API の翻訳器を作る。apiKey が空なら SDK が環境変数などから探す。
func NewAnthropic(model, apiKey, effort string, names LangNamer) *Anthropic {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &Anthropic{Client: anthropic.NewClient(opts...), Model: model, Effort: effort, Names: names}
}

// Params は API に送る内容を組み立てる (テスト用に公開)。
// temperature は今のモデルでは受け付けないので送らない。
func (c *Anthropic) Params(r Request) anthropic.BetaMessageNewParams {
	var msgs []anthropic.BetaMessageParam
	for _, h := range r.History {
		msgs = append(msgs,
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(h.Source)),
			anthropic.BetaMessageParam{OfAssistant: &anthropic.BetaAssistantMessageParam{
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(h.Target)}}})
	}
	msgs = append(msgs, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(r.Text)))
	p := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(c.Model),
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: SystemPrompt(r, c.Names)}},
		Messages:  msgs,
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
	}
	if c.Effort != "" {
		p.OutputConfig = anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffort(c.Effort)}
	}
	return p
}

// Translate は 1 文を翻訳する。
func (c *Anthropic) Translate(ctx context.Context, r Request) (string, error) {
	resp, err := c.Client.Beta.Messages.New(ctx, c.Params(r))
	if err != nil {
		var apierr *anthropic.Error
		if errors.As(err, &apierr) {
			return "", fmt.Errorf("llm: Claude API HTTP %d: %s", apierr.StatusCode, truncate(apierr.Error(), 300))
		}
		return "", err
	}
	switch resp.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return "", fmt.Errorf("llm: Claude が翻訳を断りました (%s)", resp.StopDetails.Category)
	case anthropic.BetaStopReasonMaxTokens:
		return "", fmt.Errorf("llm: 出力が max_tokens で切れました")
	}
	var out strings.Builder
	for _, b := range resp.Content {
		if t, ok := b.AsAny().(anthropic.BetaTextBlock); ok {
			out.WriteString(t.Text)
		}
	}
	return strings.TrimSpace(out.String()), nil
}

// Health は API キーが使えるかをモデル情報の取得で確かめる (トークンを消費しない)。
func (c *Anthropic) Health(ctx context.Context) error {
	if _, err := c.Client.Models.Get(ctx, c.Model, anthropic.ModelGetParams{}); err != nil {
		return fmt.Errorf("llm: Claude API に接続できません: %w", err)
	}
	return nil
}
