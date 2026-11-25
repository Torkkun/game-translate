package check

import (
	"strings"
	"testing"

	"github.com/Torkkun/game-translate/internal/unit"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name, src, dst string
		c              unit.Constraints
		want           string // 警告に含まれるべき文字列。空なら警告なし
	}{
		{"ok", "Hello, {0}!", "こんにちは、{0}!", unit.Constraints{}, ""},
		{"empty", "Hello", "  ", unit.Constraints{}, "空"},
		{"placeholder missing", "Found {0} clues", "手がかりを見つけた", unit.Constraints{}, "不足 {0}"},
		{"tag extra", "Press <b>E</b>", "<b>E</b>を押す<br>", unit.Constraints{}, "余分 <br>"},
		{"printf", "%d items", "%d 個", unit.Constraints{}, ""},
		{"newline", "A\nB", "AB", unit.Constraints{}, "改行数"},
		{"maxlen", "Save", "セーブする", unit.Constraints{MaxLen: 3}, "長すぎる"},
		{"untranslated same", "Continue", "Continue", unit.Constraints{}, "未翻訳"},
		{"untranslated english", "Where were you?", "Where were you last night?", unit.Constraints{}, "未翻訳"},
		{"proper noun ok", "Talk to Gus", "Gusと話す", unit.Constraints{}, ""},
		{"control token", "Yes", "はい<|plamo:tag|>", unit.Constraints{}, "制御トークン"},
	}
	// 原語維持の固有名詞だけの訳文は未翻訳扱いしない
	if w := Check("Bobby Yale", "Bobby Yale", unit.Constraints{}, "Bobby Yale", "Yale"); len(w) != 0 {
		t.Fatalf("原語維持の名前だけなら警告なしのはずが %v", w)
	}
	if w := Check("Talk to Bobby Yale about it", "Talk to Bobby Yale about it", unit.Constraints{}, "Bobby Yale"); len(w) == 0 {
		t.Fatal("名前以外が英語のままなら警告が出るはず")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := strings.Join(Check(tt.src, tt.dst, tt.c), "; ")
			if tt.want == "" && w != "" {
				t.Fatalf("警告なしのはずが %q", w)
			}
			if tt.want != "" && !strings.Contains(w, tt.want) {
				t.Fatalf("%q を含む警告のはずが %q", tt.want, w)
			}
		})
	}
}
