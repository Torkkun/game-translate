package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/translate"
	"github.com/Torkkun/game-translate/internal/unit"
)

// cmdConsistency は、同じ原文に違う訳が付いている組と、用語集に無い固有名詞の候補を一覧する。ファイルは書き換えない。
func cmdConsistency(args []string) error {
	flags := flag.NewFlagSet("consistency", flag.ExitOnError)
	in := flags.String("in", "", "翻訳結果 JSONL (必須)")
	gloss := flags.String("glossary", "", "用語集 YAML (ここにある語は固有名詞の候補から外す)")
	maxWords := flags.Int("max-words", 6, "この語数以下の原文だけを比べる (UI の語・人名・短い台詞。0 なら全部)")
	minNames := flags.Int("min-names", 3, "固有名詞の候補にする最少の出現回数 (0 なら候補を出さない)")
	flags.Parse(args)
	if *in == "" {
		flags.Usage()
		return fmt.Errorf("-in は必須です")
	}
	g, err := glossary.Load(*gloss)
	if err != nil {
		return err
	}
	units, err := unit.ReadJSONL(*in)
	if err != nil {
		return err
	}
	groups := translate.Inconsistent(units, *maxWords)
	fmt.Println("## 同じ原文で訳が違う組")
	for _, gr := range groups {
		fmt.Printf("%s (%d 行)\n", gr.Source, gr.Count())
		for _, v := range gr.Variants {
			ids := v.IDs
			more := ""
			if len(ids) > 3 {
				ids, more = ids[:3], fmt.Sprintf(" ほか %d 件", len(v.IDs)-3)
			}
			fmt.Printf("  %d\t%s\t[%s%s]\n", len(v.IDs), v.Target, strings.Join(ids, " "), more)
		}
	}
	fmt.Printf("訳が違う組 %d 組\n", len(groups))
	if *minNames > 0 {
		cs := translate.NameCandidates(units, g, *minNames)
		fmt.Println("\n## 用語集に無い固有名詞の候補 (文の途中に大文字で出てくる語)")
		fmt.Println("語\t回数\t小文字\t例")
		for _, c := range cs {
			fmt.Printf("%s\t%d\t%d\t%s\n", c.Word, c.Count, c.Lower, c.ID)
		}
		fmt.Printf("候補 %d 語 (人名・地名・艦名などは用語集に訳を書いてから訳す)\n", len(cs))
	}
	return nil
}
