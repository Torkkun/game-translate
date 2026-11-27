package translate

import (
	"testing"

	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/unit"
)

func TestInconsistent(t *testing.T) {
	units := []unit.Unit{
		{ID: "1", Source: "Defend", Target: "防御"},
		{ID: "2", Source: "DEFEND", Target: "防御"},
		{ID: "3", Source: "Defend.", Target: "防衛"},
		{ID: "4", Source: "Resume", Target: "再開"},
		{ID: "5", Source: "Resume", Target: "再開。"},  // 句読点の違いは同じ訳
		{ID: "6", Source: "Exit scene", Target: ""}, // 訳の無い行は見ない
		{ID: "7", Source: "Exit scene", Target: "シーンを終了"},
		{ID: "8", Source: "This line is far too long to be a term", Target: "あ"},
		{ID: "9", Source: "This line is far too long to be a term", Target: "い"},
	}
	got := Inconsistent(units, 4)
	if len(got) != 1 {
		t.Fatalf("1 組のはず: %+v", got)
	}
	g := got[0]
	if g.Source != "Defend" || g.Count() != 3 || g.Variants[0].Target != "防御" || len(g.Variants[0].IDs) != 2 {
		t.Errorf("%+v", g)
	}
	if n := len(Inconsistent(units, 0)); n != 2 {
		t.Errorf("長さを問わなければ 2 組: %d", n)
	}
}

func TestNameCandidates(t *testing.T) {
	units := []unit.Unit{
		{ID: "a", Source: "Later, Hope!"},
		{ID: "b", Source: "Ask Hope about it."},
		{ID: "c", Source: "I told Hope and Red Hat."},
		{ID: "d", Source: "Red Hat is here, and Red Hat is angry."}, // 文頭の分は「Ball」になるので数えない
		{ID: "e", Source: "Show me. Show it to Ivo."},               // 文頭の Show は数えない
		{ID: "f", Source: "We need to Save, save, save the Ivo."},   // 小文字でも多い語は外す
		{ID: "g", Source: "Talk to Marshal. Talk to Marshal again. The Marshal said so."},
	}
	g, err := glossary.New([]glossary.Term{{Source: "Marshal", Target: "保安官"}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, c := range NameCandidates(units, g, 2) {
		got[c.Word] = c.Count
	}
	if got["Hope"] != 3 || got["Red Hat"] != 2 || got["Ivo"] != 2 {
		t.Errorf("%v", got)
	}
	for _, w := range []string{"Show", "Save", "Marshal", "I", "Talk"} {
		if _, ok := got[w]; ok {
			t.Errorf("%s は候補にしないはず: %v", w, got)
		}
	}
}
