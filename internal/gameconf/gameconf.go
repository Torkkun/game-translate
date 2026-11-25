// Package gameconf は games/<game>/game.yaml (ゲームごとの設定) を読む。
//
// game.yaml にはリポジトリで共有する既定値 (Steam の標準の場所など) を書き、
// 自分の PC だけの値 (ゲームを別のドライブに入れた場合の install_dir など) は
// game.local.yaml に書く (git に入れない)。game.local.yaml にある項目が優先される。
package gameconf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config はゲームごとの設定。
type Config struct {
	Name        string `yaml:"-"`
	Title       string `yaml:"title"`
	SteamAppID  int    `yaml:"steam_appid"`
	InstallDir  string `yaml:"install_dir"`
	Exe         string `yaml:"exe"`
	Translation string `yaml:"translation"`
	// ModDir は Mod のファイルを置くゲーム側のフォルダ (ゲームの外に置く訳文ファイルなど)。空なら各コマンドの既定
	ModDir string `yaml:"mod_dir"`
	// Preserve は gtl uninstall で消す前に退避するファイルの名前 (プレイ中にできる記録・プラグインの設定など)
	Preserve []string `yaml:"preserve"`
}

// Load は games/<name>/game.yaml と、あれば game.local.yaml を読む。
func Load(name string) (*Config, error) {
	dir := filepath.Join("games", name)
	var c Config
	found := false
	for _, f := range []string{"game.yaml", "game.local.yaml"} {
		p := filepath.Join(dir, f)
		b, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		// 2 つ目のファイルは、書かれている項目だけを上書きする
		if err := yaml.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		found = true
	}
	if !found {
		return nil, fmt.Errorf("ゲーム設定が読めません: %s/game.yaml がありません", dir)
	}
	c.Name = name
	return &c, nil
}

var wslMount = regexp.MustCompile(`^/mnt/([a-zA-Z])(/.*)?$`)

// WindowsPath は WSL のパス (/mnt/g/...) を Windows のパス (G:/...) にする。区切りは / のまま。
// それ以外のパスはそのまま返す。
func WindowsPath(p string) string {
	m := wslMount.FindStringSubmatch(p)
	if m == nil {
		return p
	}
	rest := m[2]
	if rest == "" {
		rest = "/"
	}
	return strings.ToUpper(m[1]) + ":" + rest
}
