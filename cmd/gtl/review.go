package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/translate"
	"github.com/Torkkun/game-translate/internal/unit"
)

// cmdRecheck はモデルを呼ばずに、用語集の別名置き換えと検証をやり直す。
func cmdRecheck(args []string) error {
	flags := flag.NewFlagSet("recheck", flag.ExitOnError)
	in := flags.String("in", "", "翻訳結果 JSONL (上書きする)")
	gloss := flags.String("glossary", "", "用語集 YAML")
	speakersPath := flags.String("speakers", "", "話者プロフィール YAML")
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
	applied := 0
	for i, u := range units {
		t, ok := fix[u.ID]
		if !ok {
			continue
		}
		if t != "=" {
			units[i].Target = t
		}
		units[i].Status = unit.StatusReviewed
		units[i].Warnings = nil
		applied++
		delete(fix, u.ID)
	}
	for id := range fix {
		log.Printf("警告: %s は翻訳結果にありません", id)
	}
	if err := unit.WriteJSONL(*in, units); err != nil {
		return err
	}
	log.Printf("修正を %d 件取り込みました", applied)
	return nil
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
