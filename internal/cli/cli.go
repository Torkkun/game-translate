// Package cli は gtl のコマンドを作るための共通部品。
//
// ゲーム固有のコマンド (cmd/gtl/game_*.go) は init で Register を呼んで gtl に加わる。
// ゲーム固有のファイルが無くても gtl は共通のコマンドだけで動く。
package cli

import (
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/Torkkun/game-translate/internal/config"
)

// Command はゲーム固有のコマンド。
type Command struct {
	Name    string
	Summary string
	Run     func(args []string) error
}

var commands = map[string]Command{}

// Register はゲーム固有のコマンドを登録する (cmd/gtl/game_*.go の init から呼ぶ)。
func Register(name, summary string, run func([]string) error) {
	commands[name] = Command{Name: name, Summary: summary, Run: run}
}

// Lookup は登録されたコマンドを引く。
func Lookup(name string) (Command, bool) {
	c, ok := commands[name]
	return c, ok
}

// Commands は登録されたコマンドを名前順に返す。
func Commands() []Command {
	out := make([]Command, 0, len(commands))
	for _, c := range commands {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LangFlags は言語とフォントの設定だけを使うコマンド (各ゲームの build など) のオプション。
type LangFlags struct {
	config *string
	target *string
}

// AddLangFlags は -config と -target を足す。
func AddLangFlags(fs *flag.FlagSet) *LangFlags {
	return &LangFlags{
		config: fs.String("config", "", "全体設定ファイル (既定: 環境変数 GTL_CONFIG か gtl.yaml)"),
		target: fs.String("target", "", "訳文の言語コード (既定: gtl.yaml の target_lang)"),
	}
}

// Load は設定を読み、-target で上書きする。
func (f *LangFlags) Load() (*config.Config, error) {
	c, err := config.Load(*f.config)
	if err != nil {
		return nil, err
	}
	if *f.target != "" && *f.target != c.TargetLang {
		c.TargetLang = *f.target
		c.Fonts.System = config.DefaultSystemFonts(c.TargetLang)
	}
	return c, nil
}

// OrDefault は v が空なら def を返す。
func OrDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// CopyTree は src の中身を dst にコピーする。
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return CopyFile(p, target)
	})
}

// CopyFile は src を dst にコピーする (フォルダは作る)。
func CopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
