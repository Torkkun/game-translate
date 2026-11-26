package translate

import (
	"regexp"
	"strings"
)

// 男言葉の検出 (日本語の訳文用)。
//
// キャラクター表の avoid / avoid_endings はゲームごとに書く必要があり、書き漏れると
// 「あいつのことなんて知ったことか。満足か？」のような訳 (女性の話者) を素通りさせてしまう。
// そこで既に訳した分の点検 (gtl speakers-check -masculine) では、gender: female の話者について、ここにある男性的な言い方も調べる。
// 翻訳の指示・自動の訳し直しには使わない (新しく訳す分はキャラクター表とエージェントの直接翻訳で防ぐ)。
// 乱暴な口調・男勝りのキャラクターは、表の allow に許す言い方の名前 (下の name) を書くか、
// allow: [男言葉] で全部を外す。

// masculineMarker は男性的な言い方 1 種類。
type masculineMarker struct {
	name string                                // 警告と allow に使う名前
	re   *regexp.Regexp                        // 候補。マッチの前後の文字列で ok を見る
	ok   func(before, match, rest string) bool // true なら男言葉ではない (除外)
}

// AllowAllMasculine は allow に書くと男言葉の検出を全部外す名前。
const AllowAllMasculine = "男言葉"

// 文の終わり (語尾の直後に来るもの)。sentenceEnd と同じだが、伸ばし・繰り返しの記号も許す。
const toneEnd = `[ーっッ〜]*(?:[。！？!?…‥」』）)\s、]|$)`

func hasAnySuffix(s string, xs ...string) bool {
	for _, x := range xs {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}

var masculineMarkers = []masculineMarker{
	{name: "俺", re: regexp.MustCompile(`俺|オレ様`)},
	{name: "お前", re: regexp.MustCompile(`お前|おまえ|てめえ|てめぇ|テメエ|テメェ|貴様`)},
	{
		name: "あいつ", re: regexp.MustCompile(`[あこそど]いつ`),
		// 「あそこ/そこ/どこ + いつも」などを外す
		ok: func(before, m, rest string) bool {
			return strings.HasPrefix(m, "こ") && hasAnySuffix(before, "そ", "ど", "こ")
		},
	},
	{name: "野郎・クソ", re: regexp.MustCompile(`野郎|クソ|くそ[っ、！!。]|^くそ`)},
	{
		name: "〜ぜ・〜ぞ", re: regexp.MustCompile(`[ぜぞ]` + toneEnd),
		ok: func(before, m, rest string) bool { return hasAnySuffix(before, "どう", "な", "それ", "ど") }, // どうぞ・なぜ・それぞ(れ)・など
	},
	{
		name: "〜だな・〜するな", re: regexp.MustCompile(`(?:(?:だ|よ|る|た|ねえ)な|[うくすつぬむぶぐる]なよ)` + toneEnd),
		// 「〜かな」は検出しない (正規表現に含めない)。「〜ような」「〜みたいな」は文の途中なので文末で切れない
	},
	{
		name: "〜か？", re: regexp.MustCompile(`か[？?！!。…]|か$`),
		// 丁寧形の疑問 (ですか・ますか・でしょうか)・疑問詞 (何か・誰か) は外す
		ok: func(before, m, rest string) bool {
			// 感嘆の「か」(どれだけ〜か！・どんなに〜か)
			sent := before[strings.LastIndexAny(before, "。！？!?")+1:]
			for _, w := range []string{"どれだけ", "どんなに", "どれほど", "何度", "なんと", "いかに"} {
				if strings.Contains(sent, w) {
					return true
				}
			}
			if m == "か。" && strings.HasSuffix(before, "の") {
				return true // 間接疑問 (どうして〜なのか。)
			}
			return hasAnySuffix(before, "いう", "まあいい", "まさ", "です", "ます", "ません", "ました", "でした", "でしょう", "ましょう",
				"よう", "ろう", "こう", "おう", "何", "なに", "誰", "だれ", "どこ", "いつ", "どれ", "なん", "いくつ", "いくら", "ほか", "他", "わず", "確", "ば")
		},
	},
	{
		name: "〜んだ？・〜のか", re: regexp.MustCompile(`(?:んだ|のか)(?:[？?]|ぜ)|のかよ|んだ[！!]`),
		ok: func(before, m, rest string) bool {
			return strings.HasPrefix(m, "んだ") && hasAnySuffix(before, "そうな", "ほんと")
		},
	},
	{name: "〜だろ", re: regexp.MustCompile(`だろ` + toneEnd)}, // 「〜んだろう？」(独り言)・「〜だろう。」(推量) は拾わない
	{name: "〜かよ", re: regexp.MustCompile(`かよ` + toneEnd)},
	{
		name: "〜ねえ (否定)", re: regexp.MustCompile(`[らかわゃさたまないけ]ねえ`),
		ok: func(before, m, rest string) bool {
			return hasAnySuffix(before+m, "いいねえ", "すごいねえ", "かわいいねえ")
		},
	},
	{name: "〜やがる", re: regexp.MustCompile(`やが[るれっ]`)},
	{
		name: "命令形 (〜しろ・〜やれ)",
		ok:   func(before, m, rest string) bool { return strings.HasSuffix(before, "何") }, // 何しろ
		re:   regexp.MustCompile(`(?:しろ|やれ|来い|待て|黙れ|見ろ|言え|行け|失せろ|寄こせ|よこせ|どけ|やめろ|止めろ|消えろ|聞け|答えろ|急げ|逃げろ|伏せろ|見せろ|任せろ|貸せ|出せ|離せ|放せ|来るな|するな|言うな)` + toneEnd),
	},
	{name: "おい", re: regexp.MustCompile(`(?:^|[」。！？!?\s])おい[、！!…]`)},
}

// masculineWarnings は訳文に含まれる男言葉の名前を返す (重複なし、出た順)。
// allow に書かれた名前は外す。skipFirstPerson のときは「俺」を外す (一人称の確認と重ねない)。
func masculineWarnings(target string, allow []string, skipFirstPerson bool) []string {
	if contains(allow, AllowAllMasculine) {
		return nil
	}
	var found []string
	for _, mk := range masculineMarkers {
		if contains(allow, mk.name) || (skipFirstPerson && mk.name == "俺") {
			continue
		}
		for _, loc := range mk.re.FindAllStringIndex(target, -1) {
			if mk.ok != nil && mk.ok(target[:loc[0]], target[loc[0]:loc[1]], target[loc[1]:]) {
				continue
			}
			found = append(found, mk.name)
			break
		}
	}
	return found
}

// MasculineMarkerNames は allow に書ける名前の一覧 (説明・テスト用)。
func MasculineMarkerNames() []string {
	out := []string{AllowAllMasculine}
	for _, mk := range masculineMarkers {
		out = append(out, mk.name)
	}
	return out
}
