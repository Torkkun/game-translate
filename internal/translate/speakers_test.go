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
    allow: [男言葉]
  Sova:
    gender: female
    note: 荒っぽい姐御。「〜だろ」「あんた」は使うが、俺・ぜ は使わない
    first_person: あたし
    allow: [〜だろ, 命令形 (〜しろ・〜やれ)]
  Sonia:
    gender: female
    note: 被害者の娘。気丈だが丁寧
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

// 女性の話者の男言葉 (-masculine。キャラクター表に avoid を書いていなくても拾う)。
// 例は、PLaMo の訳 (拾うべき) と直した訳 (拾ってはいけない) の組。
func TestMasculineMarkers(t *testing.T) {
	sp := loadSheet(t)
	bad := []string{
		"あいつのことなんて知ったことか。満足か？",
		"言ってやれ、Blacksad！あいつのあの分厚い頭には話が通じないんだ！",
		"どうやって知れっていうんだ？",
		"くそ、あの野郎、大嫌いだ！",
		"違法賭博だなんて言うなよ…。",
		"冗談だろ！",
		"あいつの角を引き抜いてやる！",
		"わかってるな？",
		"お前には関係ない。",
		"知らねえよ。",
		"何しやがる！",
		"そこで待ってろ。行くぞ！",
		"よし、これで決まりだな。",
	}
	good := []string{
		"あんな人、どうでもいいんです。……これで満足ですか？",
		"言ってやって、Blacksadさん！ この人の石頭には何を言っても通じないの！",
		"どうして私に分かるの？",
		"もう、あの男が大嫌い！",
		"違法な賭博だなんて言わないでね……",
		"冗談でしょ！",
		"角を引っこ抜いてやるわ！",
		"どうぞ、お入りください。",
		"なぜ黙っているの？",
		"そこにいつもの店があるわ。",
		"何か？",
		"それぞれ違うのよ。",
		"雨が降るかな。",
		"そうなんだ！ 知らなかった。",
		"いいねえ、それ。",
		"約束を守ってね。",
		"これでいいでしょうか。",
		"誰か来て！",
		"どうして誰かがこれを残したんだろう？",
		"彼女ならきっとそれを選んだだろう。",
		"森で会ったんだよ。",
		"なんていうか……好奇心が旺盛なのよ。",
		"まあいいか。",
		"どうして車椅子に乗ることになったのか。",
		"何しろ、私たちが夢見る革命は遠いのよ。",
		"勉強になるなあ。",
	}
	chk := func(s string, p Speaker) []string { return checkSpeakerOpt(s, p, true) }
	for _, s := range bad {
		if w := checkSpeaker(s, sp["Sonia"]); len(w) != 0 {
			t.Errorf("-masculine なしでは拾わないはず: %q %v", s, w)
		}
		if w := chk(s, sp["Sonia"]); len(w) == 0 {
			t.Errorf("拾えていない: %q", s)
		}
	}
	for _, s := range good {
		if w := chk(s, sp["Sonia"]); len(w) != 0 {
			t.Errorf("誤検知: %q %v", s, w)
		}
	}
	// allow に書いた言い方は許す。書いていない「俺」「ぜ」は拾う
	if w := chk("黙ってろ。あんたもそう思うだろ？", sp["Sova"]); len(w) != 0 {
		t.Errorf("Sova は「〜だろ」と命令形を許しているはず: %v", w)
	}
	if w := chk("あたしが行くぜ。", sp["Sova"]); len(w) == 0 {
		t.Errorf("Sova の「ぜ」を拾えていない: %v", w)
	}
	// 男性の話者は調べない
	if w := chk("あいつ、何しやがる！", sp["Bob"]); len(w) != 0 {
		t.Errorf("%v", w)
	}
}
