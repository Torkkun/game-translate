package main

import (
	"testing"

	"github.com/Torkkun/game-translate/internal/unit"
)

// 手修正でも書式記号のずれは警告する。英語のまま残すのは人の判断なので警告しない。
func TestReviewWarnings(t *testing.T) {
	c := unit.Constraints{}
	if w := reviewWarnings("Talk to", "%sに話しかける", c); len(w) == 0 {
		t.Errorf("原文に無い %s を拾えていない", "%s")
	}
	if w := reviewWarnings("It was raining.[She left.", `雨だった。\n彼女は去った。`, c); len(w) == 0 {
		t.Error("文字どおりの \\n を拾えていない")
	}
	if w := reviewWarnings("Hope", "Hope", c); len(w) != 0 {
		t.Errorf("英語のまま残した訳は警告しない: %v", w)
	}
	if w := reviewWarnings("Show %s", "%sを見せる", c); len(w) != 0 {
		t.Errorf("%v", w)
	}
}
