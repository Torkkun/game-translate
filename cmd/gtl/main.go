// gtl はゲーム日本語化ツールの CLI。
//
//	gtl ping                       翻訳サーバーの疎通確認と 1 文翻訳
//	gtl translate -in X -out Y     TranslationUnit の JSONL を一括翻訳
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Torkkun/game-translate/internal/cli"
	"github.com/Torkkun/game-translate/internal/config"
	"github.com/Torkkun/game-translate/internal/glossary"
	"github.com/Torkkun/game-translate/internal/llm"
	"github.com/Torkkun/game-translate/internal/tm"
	"github.com/Torkkun/game-translate/internal/translate"
	"github.com/Torkkun/game-translate/internal/unit"
)

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	name := os.Args[1]
	if name == "-h" || name == "--help" || name == "help" {
		usage()
		return
	}
	var err error
	if c, ok := coreCommands()[name]; ok {
		err = c(ctx, os.Args[2:])
	} else if g, ok := cli.Lookup(name); ok {
		err = g.Run(os.Args[2:])
	} else {
		fmt.Fprintf(os.Stderr, "不明なコマンド: %s\n\n", name)
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// coreCommands は共通のコマンド (どのゲームでも使う)。
func coreCommands() map[string]func(context.Context, []string) error {
	plain := func(f func([]string) error) func(context.Context, []string) error {
		return func(_ context.Context, a []string) error { return f(a) }
	}
	return map[string]func(context.Context, []string) error{
		"ping":           cmdPing,
		"translate":      cmdTranslate,
		"warnings":       plain(cmdWarnings),
		"recheck":        plain(cmdRecheck),
		"speakers-check": plain(cmdSpeakersCheck),
		"batch-export":   plain(cmdBatchExport),
		"batch-import":   plain(cmdBatchImport),
		"review":         plain(cmdReview),
		"pack":           plain(cmdPack),
		"unpack":         plain(cmdUnpack),
		"install":        plain(cmdInstall),
		"uninstall":      plain(cmdUninstall),
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `使い方: gtl <コマンド> [オプション]

コマンド:
  ping        翻訳モデルの疎通確認と 1 文翻訳
  translate   TranslationUnit の JSONL を一括翻訳 (中断しても再実行で続きから)
  warnings    翻訳結果のうち警告が残った行を一覧表示
  recheck     モデルを呼ばずに用語集の表記ゆれ修正と検証をやり直す
  speakers-check  訳文の口調がキャラクター表 (一人称・語尾) と合わない行を一覧する (手修正済みの行も)
  batch-export    エージェント (Claude など) が直接訳す行を、文脈付きの作業ファイル (JSON) に書き出す
  batch-import    エージェントが訳した作業ファイルを翻訳結果に取り込み、自動チェックにかける
  review      人が確認・修正した訳 (fixes.tsv) を取り込む
  pack        翻訳結果から原文を除いた訳文ファイル (<言語>.tsv) を作る
  unpack      抽出した原文に、訳文ファイルの訳を合わせる
  install     dist/<game> の Mod をゲームフォルダに入れる (入れたファイルを記録)
  uninstall   install で入れた Mod を外す (上書きしたファイルは元に戻す)
`)
	if cs := cli.Commands(); len(cs) > 0 {
		fmt.Fprint(os.Stderr, "\nゲーム固有のコマンド:\n")
		for _, c := range cs {
			fmt.Fprintf(os.Stderr, "  %-18s %s\n", c.Name, c.Summary)
		}
	}
	fmt.Fprint(os.Stderr, "\n各コマンドの詳細は gtl <コマンド> -h\n")
}

func cmdPing(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ping", flag.ExitOnError)
	lf := addLLMFlags(fs)
	text := fs.String("text", "The rain hasn't stopped for three days.", "試しに訳す英文")
	fs.Parse(args)

	conf, err := lf.load()
	if err != nil {
		return err
	}
	c := backend(conf)
	if err := c.Health(ctx); err != nil {
		return fmt.Errorf("翻訳モデルに接続できません (%s %s): %w", conf.LLM.Provider, conf.LLM.URL, err)
	}
	t0 := time.Now()
	out, err := c.Translate(ctx, llm.Request{Text: *text, SourceLang: conf.SourceLang, TargetLang: conf.TargetLang})
	if err != nil {
		return err
	}
	fmt.Printf("[%s / %s / %s→%s]\n%s\n→ %s\n(%.2fs)\n", conf.LLM.Provider, conf.LLM.Model, conf.SourceLang, conf.TargetLang, *text, out, time.Since(t0).Seconds())
	return nil
}

func cmdTranslate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("translate", flag.ExitOnError)
	in := fs.String("in", "", "入力 JSONL (必須)")
	out := fs.String("out", "", "出力 JSONL (省略時は入力名に .<訳文の言語> を付ける)。既にあれば訳済みの行は飛ばす")
	gloss := fs.String("glossary", "", "用語集 YAML")
	tmPath := fs.String("tm", "work/tm.sqlite", "翻訳メモリ (空文字で使わない)")
	noReuse := fs.Bool("no-tm-reuse", false, "翻訳メモリの訳を再利用しない (保存はする)")
	lf := addLLMFlags(fs)
	form := fs.String("form", "", "文体: plain (だ・である) / polite (です・ます) / 空 (指定なし)")
	workers := fs.Int("workers", 0, "同時に訳すシーン数 (llama.cpp ではスロット数以下。0 なら gtl.yaml の llm.workers か 3)")
	history := fs.Int("history", 6, "履歴として渡す直前の行数")
	limit := fs.Int("limit", 0, "今回訳す件数の上限 (試験翻訳用, 0 で全件)")
	attempts := fs.Int("attempts", 2, "警告が出たときに試す回数の上限 (最初の 1 回を含む)")
	retryWarned := fs.Bool("retry-warned", false, "前回の出力で警告が残った行だけ訳し直す")
	speakersPath := fs.String("speakers", "", "話者プロフィール YAML (characters: {名前: {note, first_person}})")
	noRoutes := fs.Bool("no-routes", false, "gtl.yaml の routes を使わず、すべての行を llm で訳す (エージェントに回す行も)")
	fs.Parse(args)

	if *in == "" {
		fs.Usage()
		return fmt.Errorf("-in は必須です")
	}
	conf, err := lf.load()
	if err != nil {
		return err
	}
	if *out == "" {
		*out = strings.TrimSuffix(*in, ".jsonl") + conf.Suffix() + ".jsonl"
	}
	if *workers == 0 {
		*workers = conf.LLM.Workers
	}
	if *workers == 0 {
		*workers = 3
	}
	if *form != "" && conf.TargetLang != "ja" {
		return fmt.Errorf("-form は訳文が日本語 (ja) のときだけ使えます")
	}
	if *form != "" && *form != "plain" && *form != "polite" {
		return fmt.Errorf("-form は plain か polite です")
	}

	g, err := glossary.Load(*gloss)
	if err != nil {
		return err
	}
	speakers, err := translate.LoadSpeakers(*speakersPath)
	if err != nil {
		return err
	}
	c := backend(conf)
	if err := c.Health(ctx); err != nil {
		return fmt.Errorf("翻訳モデルに接続できません (%s %s): %w", conf.LLM.Provider, conf.LLM.URL, err)
	}
	var route func(unit.Unit) (llm.Translator, bool)
	if !*noRoutes && len(conf.Routes) > 0 {
		route, err = buildRoute(ctx, conf, c)
		if err != nil {
			return err
		}
	}

	var mem *tm.TM
	if *tmPath != "" {
		mem, err = tm.Open(*tmPath)
		if err != nil {
			return err
		}
		defer mem.Close()
	}

	r := &translate.Runner{
		LLM:      c,
		Route:    route,
		TM:       mem,
		Glossary: g,
		Logf:     log.Printf,
		Opt: translate.Options{
			In: *in, Out: *out,
			SourceLang: conf.SourceLang, TargetLang: conf.TargetLang, Form: *form,
			Model:   conf.LLM.Model,
			Workers: *workers, History: *history, Limit: *limit,
			UseTM:       !*noReuse,
			Retries:     3,
			Speakers:    speakers,
			Attempts:    *attempts,
			RetryWarned: *retryWarned,
		},
	}
	st, err := r.Run(ctx)
	log.Printf("完了: 入力 %d / 訳済みで飛ばした %d / モデルで訳した %d / 翻訳メモリから %d / 警告あり %d / 失敗 %d / エージェントに回した %d / %.1f 秒",
		st.Total, st.Skipped, st.Translated, st.FromTM, st.Warned, st.Failed, st.Deferred, st.Elapsed.Seconds())
	log.Printf("出力: %s", *out)
	if st.Deferred > 0 {
		log.Printf("エージェントに回した行: gtl batch-export -in %s -out %s で作業ファイルを作り、訳したら gtl batch-import で取り込む", *in, *out)
	}
	if st.Warned > 0 {
		log.Printf("警告の一覧: gtl warnings -in %s", *out)
	}
	return err
}

// buildRoute は gtl.yaml の routes から、行ごとに翻訳器を選ぶ関数を作る。使うエンジンには先に接続を確かめる。
func buildRoute(ctx context.Context, conf *config.Config, def llm.Backend) (func(unit.Unit) (llm.Translator, bool), error) {
	engines := map[string]llm.Translator{config.EngineDefault: def}
	for _, name := range conf.Routes {
		if _, ok := engines[name]; ok || name == config.EngineAgent {
			continue
		}
		l, ok := conf.Engine(name)
		if !ok {
			return nil, fmt.Errorf("routes のエンジン %q が models にありません", name)
		}
		if l.Provider == "anthropic" && l.APIKey() == "" {
			return nil, fmt.Errorf("環境変数 %s に API キーを入れてください (models.%s で Claude API を使うため)", l.APIKeyEnv, name)
		}
		b := backendFor(conf, l)
		if err := b.Health(ctx); err != nil {
			return nil, fmt.Errorf("翻訳モデル %s に接続できません (%s %s): %w", name, l.Provider, l.URL, err)
		}
		engines[name] = b
	}
	return func(u unit.Unit) (llm.Translator, bool) {
		name := conf.Route(u.Context.Kind)
		if name == config.EngineAgent {
			return nil, true
		}
		return engines[name], false
	}, nil
}

func cmdWarnings(args []string) error {
	fs := flag.NewFlagSet("warnings", flag.ExitOnError)
	in := fs.String("in", "", "翻訳結果の JSONL (必須)")
	fs.Parse(args)
	if *in == "" {
		fs.Usage()
		return fmt.Errorf("-in は必須です")
	}
	units, err := unit.ReadJSONL(*in)
	if err != nil {
		return err
	}
	n := 0
	for _, u := range units {
		if len(u.Warnings) == 0 {
			continue
		}
		n++
		fmt.Printf("%s\n  原文: %s\n  訳文: %s\n  警告: %s\n\n", u.ID, u.Source, u.Target, strings.Join(u.Warnings, "; "))
	}
	fmt.Printf("警告あり %d 件 / 全 %d 件\n", n, len(units))
	return nil
}
