package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Torkkun/game-translate/internal/unit"
)

// 公開用の訳文ファイル (<言語>.tsv)。
//
// 翻訳結果の JSONL には原文 (ゲームの英文) が入っていて公開できないので、
// 「ID・原文のハッシュ・状態・訳文」だけを TSV にして共有する。使う人は自分のゲームから
// 原文を抽出し、gtl unpack で訳文を合わせて JSONL に戻す。
//
//	# コメント
//	ID <TAB> 原文のハッシュ <TAB> 状態 <TAB> 訳文 (改行は \n、タブは \t、\ は \\)

func cmdPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	in := fs.String("in", "", "翻訳結果 JSONL (必須)")
	out := fs.String("out", "", "書き出す TSV (必須。例 games/<game>/ja.tsv)")
	title := fs.String("title", "", "先頭のコメントに書くゲーム名など")
	fs.Parse(args)
	if *in == "" || *out == "" {
		fs.Usage()
		return fmt.Errorf("-in と -out は必須です")
	}
	units, err := unit.ReadJSONL(*in)
	if err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	if *title != "" {
		fmt.Fprintf(w, "# %s\n", *title)
	}
	fmt.Fprintln(w, "# gtl pack が作った訳文ファイル (原文は含まない)。使い方: gtl unpack -src <抽出した原文 JSONL> -tsv このファイル -out <翻訳結果 JSONL>")
	fmt.Fprintln(w, "# ID\t原文のハッシュ\t状態\t訳文")
	n := 0
	for _, u := range units {
		if !u.Status.Done() || u.Target == "" {
			continue
		}
		h := u.Hash
		if h == "" {
			h = unit.SourceHash(u.Source)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.ID, h, u.Status, escapeTSV(u.Target))
		n++
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	log.Printf("訳文 %d / %d 件を書き出しました → %s", n, len(units), *out)
	return nil
}

func cmdUnpack(args []string) error {
	fs := flag.NewFlagSet("unpack", flag.ExitOnError)
	src := fs.String("src", "", "自分のゲームから抽出した原文 JSONL (必須)")
	tsv := fs.String("tsv", "", "gtl pack で作った訳文 TSV (必須)")
	out := fs.String("out", "", "書き出す翻訳結果 JSONL (必須)")
	fs.Parse(args)
	if *src == "" || *tsv == "" || *out == "" {
		fs.Usage()
		return fmt.Errorf("-src と -tsv と -out は必須です")
	}
	tr, err := readPacked(*tsv)
	if err != nil {
		return err
	}
	units, err := unit.ReadJSONL(*src)
	if err != nil {
		return err
	}
	applied, stale := 0, 0
	for i, u := range units {
		p, ok := tr[u.ID]
		if !ok {
			continue
		}
		h := u.Hash
		if h == "" {
			h = unit.SourceHash(u.Source)
			units[i].Hash = h
		}
		if p.hash != h {
			// 原文が変わっている (ゲームの版が違う)。訳は入れずに再翻訳の対象にする
			stale++
			continue
		}
		units[i].Target = p.target
		units[i].Status = unit.Status(p.status)
		applied++
	}
	if err := unit.WriteJSONL(*out, units); err != nil {
		return err
	}
	log.Printf("訳文 %d / %d 件を入れました (原文が違うので入れなかった %d 件、訳文ファイルにあるが原文に無い %d 件) → %s",
		applied, len(units), stale, len(tr)-applied-stale, *out)
	if applied < len(units) {
		log.Printf("訳の無い行は gtl translate -in %s で訳せます (訳済みの行は飛ばします)", *out)
	}
	return nil
}

type packed struct{ hash, status, target string }

func readPacked(path string) (map[string]packed, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[string]packed{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimRight(sc.Text(), "\r")
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		parts := strings.SplitN(s, "\t", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("%s:%d: 「ID<TAB>ハッシュ<TAB>状態<TAB>訳文」の形ではありません", path, line)
		}
		m[parts[0]] = packed{hash: parts[1], status: parts[2], target: unescapeTSV(parts[3])}
	}
	return m, sc.Err()
}

func escapeTSV(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`).Replace(s)
}

func unescapeTSV(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
