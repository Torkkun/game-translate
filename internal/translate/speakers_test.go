package translate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sheetYAML = `
defaults:
  female:
    avoid: [俺, 僕]
    avoid_endings: [だぜ, ぜ]
characters:
  Alice:
    gender: female
    note: 皮肉屋の学生
    first_person: 私
    second_person: あなた
    speech: くだけた女性口調
    examples: [ちょっと待ってよ。]
    aliases: [ALIC]
  Rose:
    gender: female
    note: 荒っぽい運び屋。乱暴な言い方をする
    first_person: あたし
    avoid: []
    avoid_endings: []
  Bob:
    gender: male
    first_person: 俺
`

func loadSheet(t *testing.T) map[string]Speaker {
	t.Helper()
	p := filepath.Join(t.TempDir(), "characters.yaml")
	if err := os.WriteFile(p, []byte(sheetYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sp, err := LoadSpeakers(p)
	if err != nil {
		t.Fatal(err)
	}
	return sp
}

func TestLoadSpeakersDefaultsAndAliases(t *testing.T) {
	sp := loadSheet(t)
	if a := sp["ALIC"]; a.Name != "Alice" || len(a.AvoidEndings) != 2 {
		t.Fatalf("別名と既定が入っていない: %+v", a)
	}
	// キャラクター側で空にしたら既定を使わない (乱暴な口調のキャラクター)
	if r := sp["Rose"]; len(r.Avoid) != 0 || len(r.AvoidEndings) != 0 {
		t.Fatalf("Rose は既定を使わないはず: %+v", r)
	}
	// male には既定が無い
	if b := sp["Bob"]; b.Avoid != nil {
		t.Fatalf("%+v", b)
	}
}

func TestCheckSpeaker(t *testing.T) {
	sp := loadSheet(t)
	cases := []struct {
		who, target string
		want        int
	}{
		{"Alice", "ちょっと待ってよ。", 0},
		{"Alice", "俺に任せろよ。", 1},     // avoid の「俺」(一人称の警告とは重ねない)
		{"Alice", "行くぜ！", 1},        // 語尾
		{"Alice", "それぞれ違う。", 0},     // 「ぞ」は avoid にないし、「ぜ」も文末でない
		{"Alice", "かぜをひいた。", 0},     // 語尾は文の終わりだけで見る
		{"Rose", "あたしに任せな。行くぜ！", 0}, // 表で許している
		{"Rose", "俺が行く。", 1},        // 一人称が違う
		{"Bob", "俺が行くぜ。", 0},
	}
	for _, c := range cases {
		w := checkSpeaker(c.target, sp[c.who])
		if len(w) != c.want {
			t.Errorf("%s %q: %v", c.who, c.target, w)
		}
	}
}

func TestDescribe(t *testing.T) {
	d := loadSheet(t)["Alice"].Describe()
	for _, want := range []string{"女性。", "一人称は「私」", "「あなた」", "くだけた女性口調", "使わない言い方", "「だぜ」", "ちょっと待ってよ。"} {
		if !strings.Contains(d, want) {
			t.Errorf("%q が無い: %s", want, d)
		}
	}
}
