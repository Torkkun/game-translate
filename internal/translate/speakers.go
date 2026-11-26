package translate

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Speaker は話者プロフィール (キャラクター表の 1 人分)。
//
// 口調のずれ (女性キャラクターが男性的な一人称・語尾で話すなど) を防ぐため、翻訳の指示に渡し、訳文の確認にも使う。
// 決めるのはこの表で、性別だけで機械的に決めない (ワイルドな性格なら女性でも乱暴な言い方をしてよい)。
type Speaker struct {
	Name         string   `yaml:"name"`          // 表示名 (キーが音声ファイルの略号などのとき)
	Gender       string   `yaml:"gender"`        // female / male / other / 空 (不明)
	Note         string   `yaml:"note"`          // 立場・性格など (モデルへの文脈)
	FirstPerson  string   `yaml:"first_person"`  // 一人称 (例 "私")。違う一人称が出たら警告する
	SecondPerson string   `yaml:"second_person"` // 相手の呼び方 (例 "あなた"、"あんた")
	Speech       string   `yaml:"speech"`        // 話し方・語尾 (例 "くだけた女性口調。〜よ、〜ね。男言葉は使わない")
	Avoid        []string `yaml:"avoid"`         // 使わない言い方 (文中のどこでも)。例 ["俺", "僕"]
	AvoidEndings []string `yaml:"avoid_endings"` // 使わない語尾 (文の終わりだけで見る)。例 ["だぜ", "ぜ", "ぞ"]
	Allow        []string `yaml:"allow"`         // speakers-check -masculine で、gender: female でも許す男言葉の名前 (tone.go)。例 ["お前", "〜だろ"]、全部許すなら ["男言葉"]
	Examples     []string `yaml:"examples"`      // 口調の例 (訳文の例)
	Aliases      []string `yaml:"aliases"`       // 同じ人物を指す別の話者名 (context.speaker の表記ゆれ・音声の略号)
}

// SpeakerSheet はキャラクター表 (characters.yaml)。
//
//	defaults:            # gender ごとの既定 (キャラクター側に書いた項目が優先)
//	  female: {avoid_endings: [だぜ, ぜ]}
//	characters:
//	  Kathy: {gender: female, first_person: 私, speech: ..., aliases: [KATH]}
type SpeakerSheet struct {
	Defaults   map[string]Speaker `yaml:"defaults"`
	Characters map[string]Speaker `yaml:"characters"`
}

// LoadSpeakers はキャラクター表を読み、話者名 (別名を含む) → プロフィールを返す。path が空なら nil。
// gender の既定 (defaults) は、キャラクターに書いていない項目にだけ当てる。
func LoadSpeakers(path string) (map[string]Speaker, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sheet SpeakerSheet
	if err := yaml.Unmarshal(b, &sheet); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := map[string]Speaker{}
	for key, sp := range sheet.Characters {
		if d, ok := sheet.Defaults[sp.Gender]; ok {
			sp = withDefaults(sp, d)
		}
		if sp.Name == "" {
			sp.Name = key
		}
		out[key] = sp
		for _, a := range sp.Aliases {
			if _, dup := sheet.Characters[a]; !dup {
				out[a] = sp
			}
		}
	}
	return out, nil
}

func withDefaults(sp, d Speaker) Speaker {
	if sp.FirstPerson == "" {
		sp.FirstPerson = d.FirstPerson
	}
	if sp.SecondPerson == "" {
		sp.SecondPerson = d.SecondPerson
	}
	if sp.Speech == "" {
		sp.Speech = d.Speech
	}
	if sp.Avoid == nil {
		sp.Avoid = d.Avoid
	}
	if sp.AvoidEndings == nil {
		sp.AvoidEndings = d.AvoidEndings
	}
	if sp.Allow == nil {
		sp.Allow = d.Allow
	}
	return sp
}

// Describe はモデルに渡す話者の説明 (日本語)。
func (sp Speaker) Describe() string {
	var b strings.Builder
	switch sp.Gender {
	case "female":
		b.WriteString("女性。")
	case "male":
		b.WriteString("男性。")
	}
	if sp.Note != "" {
		b.WriteString(strings.TrimSpace(sp.Note))
		if !strings.HasSuffix(b.String(), "。") {
			b.WriteString("。")
		}
	}
	if sp.FirstPerson != "" {
		fmt.Fprintf(&b, " 一人称は「%s」。", sp.FirstPerson)
	}
	if sp.SecondPerson != "" {
		fmt.Fprintf(&b, " 相手を呼ぶときは「%s」。", sp.SecondPerson)
	}
	if sp.Speech != "" {
		fmt.Fprintf(&b, " 話し方: %s", strings.TrimSpace(sp.Speech))
		if !strings.HasSuffix(b.String(), "。") {
			b.WriteString("。")
		}
	}
	if avoid := append(append([]string(nil), sp.Avoid...), sp.AvoidEndings...); len(avoid) > 0 {
		fmt.Fprintf(&b, " 使わない言い方: %s。", strings.Join(quote(avoid), "、"))
	}
	if len(sp.Examples) > 0 {
		fmt.Fprintf(&b, " 口調の例: %s", strings.Join(quote(sp.Examples), " "))
	}
	return strings.TrimSpace(b.String())
}

func quote(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = "「" + x + "」"
	}
	return out
}

// 一人称のうち、他の語の一部になりにくいもの。「私」は「私立」「私物」などと紛れるので判定に使わない。
var distinctFirstPersons = []string{"俺", "僕", "あたし"} // 「わし」は「出くわし」「そわそわし」と紛れるので使わない

// 文の終わり (語尾の直後に来るもの)
const sentenceEnd = `(?:[。！？!?…」』）)\s]|$)`

// checkSpeaker は訳文が話者のキャラクター表と合っているかを調べる。
//   - 一人称: 表の一人称と違う一人称 (俺/僕/あたし) が出ていないか
//   - avoid: 使わない言い方が出ていないか
//   - avoid_endings: 使わない語尾で文が終わっていないか
func checkSpeaker(target string, sp Speaker) []string { return checkSpeakerOpt(target, sp, false) }

// checkSpeakerOpt は checkSpeaker に加えて、masculine なら gender: female の話者の男言葉 (tone.go。あいつ・〜か？・〜だろ など。
// allow に書いたものは除く) も調べる。翻訳の指示・再試行には使わず、既に訳した分の点検 (gtl speakers-check -masculine) で使う。
func checkSpeakerOpt(target string, sp Speaker, masculine bool) []string {
	var w []string
	if sp.FirstPerson != "" {
		for _, fp := range distinctFirstPersons {
			if fp != sp.FirstPerson && strings.Contains(target, fp) && !contains(sp.Avoid, fp) {
				w = append(w, fmt.Sprintf("一人称が違う (「%s」、この話者は「%s」)", fp, sp.FirstPerson))
			}
		}
	}
	for _, a := range sp.Avoid {
		if a != "" && strings.Contains(target, a) {
			w = append(w, fmt.Sprintf("話者に合わない言い方 (「%s」、%s)", a, who(sp)))
		}
	}
	var endings []string
	for _, e := range sp.AvoidEndings {
		if e != "" && regexp.MustCompile(regexp.QuoteMeta(e)+sentenceEnd).MatchString(target) {
			endings = append(endings, e)
		}
	}
	if len(endings) > 0 {
		sort.Strings(endings)
		w = append(w, fmt.Sprintf("話者に合わない語尾 (「%s」、%s)", strings.Join(endings, "」「"), who(sp)))
	}
	if masculine && sp.Gender == "female" {
		var masc []string
		for _, m := range masculineWarnings(target, sp.Allow, sp.FirstPerson != "") {
			// avoid / avoid_endings で同じ言い方をもう警告していれば重ねない
			if !strings.Contains(strings.Join(w, " "), "「"+m+"」") {
				masc = append(masc, m)
			}
		}
		if len(masc) > 0 {
			w = append(w, fmt.Sprintf("女性の話者に男言葉 (「%s」、%s。乱暴な口調の人物なら表の allow に書く)", strings.Join(masc, "」「"), who(sp)))
		}
	}
	return w
}

func who(sp Speaker) string {
	s := "キャラクター表の " + sp.Name
	if sp.Gender != "" {
		s += " (" + sp.Gender + ")"
	}
	return s
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// CheckSpeaker は訳文がキャラクター表と合っているかを調べる (gtl speakers-check 用)。
func CheckSpeaker(target string, sp Speaker, masculine bool) []string {
	return checkSpeakerOpt(target, sp, masculine)
}
