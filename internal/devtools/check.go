package devtools

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
)

// 公開リポジトリに入れてはいけないものの決まり (CI の Guard と mage check が使う)。
var (
	// ゲームごとの設定・訳文・ゲーム由来のデータ・個人用メモ・データ用リポジトリのトップの README
	dataGlobs = []string{"games/**", "docs/private/**", ".github/README.md", "**/*.tsv"}
	// ゲーム固有の拡張
	extGlobs = []string{"cmd/gtl/game_*.go", "internal/adapter/**", "internal/unityfs/**", "internal/zf/**", "plugins/**",
		"magefiles/game_*.go"}
	// ゲームのデータ形式・実行ファイルの拡張子
	gameDataExt = []string{".pak", ".patchwad", ".locres", ".translation", ".tra", ".pck", ".assets", ".bundle", ".ufont", ".tab"}
	binaryExt   = []string{".dll", ".exe", ".so", ".dylib"}
	// 原文入りの翻訳結果は testdata/ の自作サンプルだけ許す
	jsonlOK = NewGlob("testdata/**")

	// 個人の環境のパス・連絡先らしい文字列
	personalRe = regexp.MustCompile(`/home/[a-z][a-z0-9_-]*/|C:\\Users\\|C:/Users/|/mnt/[d-z]/|@gmail\.com|@outlook\.|@yahoo\.`)
	// 上の文字列を説明・テストのために含むファイル
	personalOK = []string{"internal/devtools/*", "internal/gameconf/*"}
)

// MaxFileSize は公開リポジトリに入れてよいファイルの大きさの上限。
const MaxFileSize = 1 << 20

func anyMatch(globs []string, p string) bool {
	for _, g := range globs {
		if NewGlob(g).Match(p) {
			return true
		}
	}
	return false
}

// Classify は公開リポジトリのファイル p が入れてはいけないものなら、その理由を返す。
func Classify(p string) string {
	ext := path.Ext(p)
	switch {
	case anyMatch(dataGlobs, p):
		return "ゲームごとの設定・訳文・ゲーム由来のデータ・個人用メモ (データ用リポジトリで管理する)"
	case anyMatch(extGlobs, p):
		return "ゲーム固有の拡張 (データ用リポジトリで管理する)"
	case ext == ".jsonl" && !jsonlOK.Match(p):
		return "原文入りの翻訳結果 (データ用リポジトリで管理する)"
	case strings.HasSuffix(p, ".wad.pc") || strings.HasSuffix(p, ".big.pc") || contains(gameDataExt, ext):
		return "ゲームのデータ形式のファイル"
	case contains(binaryExt, ext):
		return "実行ファイル (ビルドしたものは入れない)"
	}
	return ""
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// CheckPublic は git で追跡しているファイルを調べ、公開してはいけないものの一覧を返す (空なら問題なし)。
func CheckPublic(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "-c", "core.quotepath=false", "ls-files", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var problems []string
	for _, p := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if p == "" {
			continue
		}
		if why := Classify(p); why != "" {
			problems = append(problems, fmt.Sprintf("%s: %s", why, p))
			continue
		}
		full := root + "/" + p
		st, err := os.Stat(full)
		if err != nil {
			continue // 消したばかりのファイル
		}
		if st.Size() > MaxFileSize {
			problems = append(problems, fmt.Sprintf("1MB を超えるファイル (%dKB): %s", st.Size()/1024, p))
			continue
		}
		if anyMatch(personalOK, p) {
			continue
		}
		hits, err := grepFile(full, personalRe)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			problems = append(problems, fmt.Sprintf("個人の環境のパスやメールアドレスらしい文字列: %s:%s", p, h))
		}
	}
	return problems, nil
}

// grepFile は re に当たる行を「行番号: 内容」で返す。バイナリは見ない。
func grepFile(path string, re *regexp.Regexp) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return nil, nil
	}
	var hits []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	n := 0
	for sc.Scan() {
		n++
		if re.Match(sc.Bytes()) {
			line := sc.Text()
			if len(line) > 120 {
				line = line[:120] + "..."
			}
			hits = append(hits, fmt.Sprintf("%d: %s", n, line))
		}
	}
	return hits, sc.Err()
}
