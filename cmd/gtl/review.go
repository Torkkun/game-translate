package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/Torkkun/game-translate/internal/check"
	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/translate"
	"github.com/Torkkun/game-translate/internal/unit"
)

// cmdRecheck はモデルを呼ばずに、用語集の別名置き換えと検証をやり直す。
func cmdRecheck(args []string) error {
	flags := flag.NewFlagSet("recheck", flag.ExitOnError)
	in := flags.String("in", "", "翻訳結果 JSONL (上書きする)")
	gloss := flags.String("glossary", "", "用語集 YAML")
	speakersPath := flags.String("speakers", "", "キャラクター表 YAML (characters.yaml)")
	contextFrom := flags.String("context-from", "", "抽出し直した原文 JSONL。話者 (context.speaker) をここから取り直す (話者の割り当てを後から足したとき)")
	flags.Parse(args)
	if *in == "" {
		flags.Usage()
		return fmt.Errorf("-in は必須です")
	}
	g, err := glossary.Load(*gloss)
	if err != nil {
		return err
	}
	sp, err := translate.LoadSpeakers(*speakersPath)
	if err != nil {
		return err
	}
	units, err := unit.ReadJSONL(*in)
	if err != nil {
		return err
	}
	if *contextFrom != "" {
		src, err := unit.ReadJSONL(*contextFrom)
		if err != nil {
			return err
		}
		speaker := map[string]string{}
		for _, u := range src {
			speaker[u.ID] = u.Context.Speaker
		}
		n := 0
		for i := range units {
			if s, ok := speaker[units[i].ID]; ok && s != units[i].Context.Speaker {
				units[i].Context.Speaker = s
				n++
			}
		}
		log.Printf("話者を %d 件取り直しました (%s)", n, *contextFrom)
	}
	r := &translate.Runner{Glossary: g, Opt: translate.Options{Speakers: sp}}
	before, after := 0, 0
	for i := range units {
		if len(units[i].Warnings) > 0 {
			before++
		}
		units[i] = r.Recheck(units[i])
		if len(units[i].Warnings) > 0 {
			after++
		}
	}
	if err := unit.WriteJSONL(*in, units); err != nil {
		return err
	}
	log.Printf("警告あり %d 件 → %d 件", before, after)
	return nil
}

// cmdReview は人が確認・修正した訳 (fixes.tsv) を翻訳結果に取り込む。
//
// fixes.tsv の書式 (1 行 1 件、# で始まる行は注釈):
//
//	ID <TAB> 訳文      … 訳文をこの内容に直す
//	ID <TAB> =         … 今の訳文で問題なし (警告を消す)
//
// 取り込んだ行は status=reviewed になり、再翻訳 (-retry-warned を含む) で上書きされない。
// 訳文を書き換えた行は書式記号・改行の数を検証し直し、問題があれば警告として残す
// (手修正で %s を足した・文字どおりの \n が入った、などを素通りさせない)。英語のまま残すのは人の判断なので「未翻訳」は警告しない。
func cmdReview(args []string) error {
	flags := flag.NewFlagSet("review", flag.ExitOnError)
	in := flags.String("in", "", "翻訳結果 JSONL (上書きする)")
	fixes := flags.String("fixes", "", "修正ファイル (ID\\t訳文 または ID\\t=)")
	flags.Parse(args)
	if *in == "" || *fixes == "" {
		flags.Usage()
		return fmt.Errorf("-in と -fixes は必須です")
	}
	fix, err := readFixes(*fixes)
	if err != nil {
		return err
	}

	units, err := unit.ReadJSONL(*in)
	if err != nil {
		return err
	}
	applied, warned := 0, 0
	for i, u := range units {
		t, ok := fix[u.ID]
		if !ok {
			continue
		}
		units[i].Status = unit.StatusReviewed
		units[i].Warnings = nil
		if t != "=" {
			units[i].Target = t
			if w := reviewWarnings(u.Source, t, u.Constraints); len(w) > 0 {
				units[i].Warnings = w
				warned++
				log.Printf("警告: %s の手修正: %s", u.ID, strings.Join(w, "; "))
			}
		}
		applied++
		delete(fix, u.ID)
	}
	for id := range fix {
		log.Printf("警告: %s は翻訳結果にありません", id)
	}
	if err := unit.WriteJSONL(*in, units); err != nil {
		return err
	}
	log.Printf("修正を %d 件取り込みました (うち警告あり %d 件。gtl warnings で一覧できる)", applied, warned)
	return nil
}

// reviewWarnings は手修正の訳文を検証する。「未翻訳に見える」は人が英語のまま残した場合があるので外す。
func reviewWarnings(source, target string, c unit.Constraints) []string {
	var out []string
	for _, w := range check.Check(source, target, c) {
		if w != "未翻訳に見える" {
			out = append(out, w)
		}
	}
	return out
}

// readFixes は fixes.tsv を読み、ID → 訳文 ("=" は「今の訳で問題なし」) を返す。
func readFixes(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fix := map[string]string{}
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimRight(sc.Text(), "\r")
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		id, text, ok := strings.Cut(s, "\t")
		if !ok || id == "" || text == "" {
			return nil, fmt.Errorf("%s:%d: 「ID<TAB>訳文」の形ではありません", path, line)
		}
		fix[id] = strings.ReplaceAll(text, `\n`, "\n")
	}
	return fix, sc.Err()
}

// cmdSpeakersCheck は、話者が分かっている行の訳文がキャラクター表 (一人称・使わない言い方・語尾) と合っているかを
// 状態 (機械翻訳・手修正済み) に関わらず一覧する。ファイルは書き換えない。
func cmdSpeakersCheck(args []string) error {
	flags := flag.NewFlagSet("speakers-check", flag.ExitOnError)
	in := flags.String("in", "", "翻訳結果 JSONL")
	speakersPath := flags.String("speakers", "", "キャラクター表 YAML (characters.yaml)")
	contextFrom := flags.String("context-from", "", "話者をここ (抽出し直した原文 JSONL) から取る (任意)")
	masculine := flags.Bool("masculine", false, "gender: female の話者の男言葉 (あいつ・お前・〜か？・〜だろ など) も調べる。表の allow は除く (既に訳した分の点検用)")
	summary := flags.Bool("summary", false, "行の一覧を出さず、話者ごと・警告の種類ごとの件数だけを出す")
	flags.Parse(args)
	if *in == "" || *speakersPath == "" {
		flags.Usage()
		return fmt.Errorf("-in と -speakers は必須です")
	}
	sp, err := translate.LoadSpeakers(*speakersPath)
	if err != nil {
		return err
	}
	units, err := unit.ReadJSONL(*in)
	if err != nil {
		return err
	}
	speaker := map[string]string{}
	if *contextFrom != "" {
		src, err := unit.ReadJSONL(*contextFrom)
		if err != nil {
			return err
		}
		for _, u := range src {
			speaker[u.ID] = u.Context.Speaker
		}
	}
	checked, bad := 0, 0
	type count struct{ checked, bad int }
	bySpeaker := map[string]*count{}
	byKind := map[string]int{}
	for _, u := range units {
		s := u.Context.Speaker
		if v, ok := speaker[u.ID]; ok {
			s = v
		}
		p, ok := sp[s]
		if !ok || u.Target == "" {
			continue
		}
		checked++
		c := bySpeaker[p.Name]
		if c == nil {
			c = &count{}
			bySpeaker[p.Name] = c
		}
		c.checked++
		if w := translate.CheckSpeaker(u.Target, p, *masculine); len(w) > 0 {
			bad++
			c.bad++
			for _, x := range w {
				for _, k := range warningKinds(x) {
					byKind[k]++
				}
			}
			if *summary {
				continue
			}
			fmt.Printf("%s [%s, %s]\n  原文: %s\n  訳文: %s\n  警告: %s\n\n", u.ID, p.Name, u.Status, u.Source, u.Target, strings.Join(w, "; "))
		}
	}
	if *summary {
		names := make([]string, 0, len(bySpeaker))
		for n := range bySpeaker {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool { return bySpeaker[names[i]].bad > bySpeaker[names[j]].bad })
		fmt.Println("話者\t合わない行\t話者の行")
		for _, n := range names {
			if c := bySpeaker[n]; c.bad > 0 {
				fmt.Printf("%s\t%d\t%d\n", n, c.bad, c.checked)
			}
		}
		kinds := make([]string, 0, len(byKind))
		for k := range byKind {
			kinds = append(kinds, k)
		}
		sort.Slice(kinds, func(i, j int) bool { return byKind[kinds[i]] > byKind[kinds[j]] })
		fmt.Println("\n警告の種類\t件数")
		for _, k := range kinds {
			fmt.Printf("%s\t%d\n", k, byKind[k])
		}
		fmt.Println()
	}
	fmt.Printf("キャラクター表と合わない行 %d 件 / 話者が分かる行 %d 件\n", bad, checked)
	return nil
}

// warningKinds は警告文から集計用の種類を取り出す。言い方が複数並んでいれば 1 つずつに分ける
// (「女性の話者に男言葉 (「あいつ」「〜か？」、…)」→「女性の話者に男言葉: あいつ」「女性の話者に男言葉: 〜か？」)。
func warningKinds(w string) []string {
	head, rest, ok := strings.Cut(w, " (")
	if !ok {
		return []string{w}
	}
	if strings.HasPrefix(head, "一人称") {
		return []string{head}
	}
	if i := strings.Index(rest, "、"); i >= 0 {
		rest = rest[:i]
	}
	var out []string
	for _, x := range strings.Split(strings.Trim(rest, "「」"), "」「") {
		out = append(out, head+": "+x)
	}
	return out
}
