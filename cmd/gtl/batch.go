package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"strings"

	"github.com/Torkkun/game-translate/internal/config"
	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/translate"
	"github.com/Torkkun/game-translate/internal/unit"
)

// batchFlags は batch-export と batch-import に共通のオプション。
type batchFlags struct {
	config, in, out, dir, gloss, speakers *string
}

func addBatchFlags(fs *flag.FlagSet) *batchFlags {
	return &batchFlags{
		config:   fs.String("config", "", "全体設定ファイル (既定: 環境変数 GTL_CONFIG か gtl.yaml)"),
		in:       fs.String("in", "", "抽出した原文の JSONL (必須。gtl translate の -in と同じ)"),
		out:      fs.String("out", "", "翻訳結果の JSONL (省略時は入力名に .<訳文の言語> を付ける。gtl translate の -out と同じ)"),
		dir:      fs.String("dir", "", "作業ファイルのフォルダ (省略時は翻訳結果の名前 + .batches)"),
		gloss:    fs.String("glossary", "", "用語集 YAML"),
		speakers: fs.String("speakers", "", "キャラクター表 YAML (characters.yaml)"),
	}
}

func (f *batchFlags) load(fs *flag.FlagSet) (*config.Config, error) {
	if *f.in == "" {
		fs.Usage()
		return nil, fmt.Errorf("-in は必須です")
	}
	conf, err := config.Load(*f.config)
	if err != nil {
		return nil, err
	}
	if *f.out == "" {
		*f.out = strings.TrimSuffix(*f.in, ".jsonl") + conf.Suffix() + ".jsonl"
	}
	if *f.dir == "" {
		*f.dir = strings.TrimSuffix(*f.out, ".jsonl") + ".batches"
	}
	return conf, nil
}

// readPrevious は翻訳結果を ID ごとに読む (まだ無ければ空)。
func readPrevious(path string) (map[string]unit.Unit, error) {
	us, err := unit.ReadJSONL(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]unit.Unit{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := make(map[string]unit.Unit, len(us))
	for _, u := range us {
		m[u.ID] = u
	}
	return m, nil
}

// done は前回の結果にこの行の訳が (原文が変わらないまま) あるか。
func done(prev map[string]unit.Unit, u unit.Unit) bool {
	p, ok := prev[u.ID]
	return ok && p.Hash == unit.SourceHash(u.Source) && p.Status.Done() && p.Target != ""
}

func cmdBatchExport(args []string) error {
	fs := flag.NewFlagSet("batch-export", flag.ExitOnError)
	bf := addBatchFlags(fs)
	form := fs.String("form", "", "文体: plain (だ・である) / polite (です・ます) / 空 (指定なし)")
	size := fs.Int("size", 40, "1 つの作業ファイルのおおよその行数 (シーンの途中ではなるべく切らない)")
	all := fs.Bool("all", false, "routes に関係なく、まだ訳の無い行をすべて書き出す")
	warned := fs.Bool("warned", false, "まだ訳の無い行に加えて、機械翻訳で警告が残った行も書き出す (訳し直し用)")
	fs.Parse(args)
	conf, err := bf.load(fs)
	if err != nil {
		return err
	}
	g, err := glossary.Load(*bf.gloss)
	if err != nil {
		return err
	}
	sp, err := translate.LoadSpeakers(*bf.speakers)
	if err != nil {
		return err
	}
	units, err := unit.ReadJSONL(*bf.in)
	if err != nil {
		return err
	}
	prev, err := readPrevious(*bf.out)
	if err != nil {
		return err
	}
	var todo []unit.Unit
	for _, u := range units {
		if !*all && conf.Route(u.Context.Kind) != config.EngineAgent {
			continue
		}
		if done(prev, u) {
			p := prev[u.ID]
			if !*warned || p.Status != unit.StatusMachine || len(p.Warnings) == 0 {
				continue
			}
		}
		if u.Status.Done() && u.Target != "" {
			continue
		}
		todo = append(todo, u)
	}
	if len(todo) == 0 {
		log.Printf("書き出す行がありません (エージェントに回す行は gtl.yaml の routes で %q を指定します。すべて書き出すなら -all)", config.EngineAgent)
		return nil
	}
	bs := translate.BatchExport(todo, translate.BatchOptions{
		SourceLang: conf.SourceLang, TargetLang: conf.TargetLang, Form: *form,
		Size: *size, Speakers: sp, Glossary: g,
	})
	paths, err := translate.WriteBatches(*bf.dir, bs)
	if err != nil {
		return err
	}
	log.Printf("%d 行を %d 個の作業ファイルに書き出しました: %s", len(todo), len(paths), *bf.dir)
	log.Printf("各ファイルの lines[].target を埋めてから: gtl batch-import -in %s -out %s", *bf.in, *bf.out)
	return nil
}

func cmdBatchImport(args []string) error {
	fs := flag.NewFlagSet("batch-import", flag.ExitOnError)
	bf := addBatchFlags(fs)
	fs.Parse(args)
	if _, err := bf.load(fs); err != nil {
		return err
	}
	g, err := glossary.Load(*bf.gloss)
	if err != nil {
		return err
	}
	sp, err := translate.LoadSpeakers(*bf.speakers)
	if err != nil {
		return err
	}
	targets, err := translate.ReadBatchTargets(*bf.dir)
	if err != nil {
		return err
	}
	units, err := unit.ReadJSONL(*bf.in)
	if err != nil {
		return err
	}
	prev, err := readPrevious(*bf.out)
	if err != nil {
		return err
	}
	r := &translate.Runner{Glossary: g, Opt: translate.Options{Speakers: sp}}
	var imported, warned, kept, matched int
	for i, u := range units {
		u.Hash = unit.SourceHash(u.Source)
		if u.Status == "" {
			u.Status = unit.StatusNew
		}
		p, hasPrev := prev[u.ID]
		t, ok := targets[u.ID]
		if ok {
			matched++
		}
		switch {
		case ok && hasPrev && (p.Status == unit.StatusReviewed || p.Status == unit.StatusLocked) && p.Hash == u.Hash:
			units[i] = p // 人が確認した訳は上書きしない
			kept++
			continue
		case ok:
			u.Target = t
			u.Status = unit.StatusMachine
			u = r.Recheck(u)
			imported++
			if len(u.Warnings) > 0 {
				warned++
			}
		case hasPrev:
			u = p
		}
		units[i] = u
	}
	if err := unit.WriteJSONL(*bf.out, units); err != nil {
		return err
	}
	log.Printf("取り込み %d 件 (うち警告あり %d 件) / 人が確認済みのため取り込まなかった %d 件 / 原文に無い ID %d 件", imported, warned, kept, len(targets)-matched)
	log.Printf("出力: %s", *bf.out)
	if warned > 0 {
		log.Printf("警告の一覧: gtl warnings -in %s", *bf.out)
	}
	rest := 0
	for _, u := range units {
		if !u.Status.Done() || u.Target == "" {
			rest++
		}
	}
	if rest > 0 {
		log.Printf("まだ訳の無い行が %d 件あります (gtl translate で訳すか、gtl batch-export で書き出し直す)", rest)
	}
	return nil
}
