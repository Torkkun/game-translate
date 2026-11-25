// Package devtools は開発用のタスク (magefiles/) が使う処理。テストできるように普通のパッケージにしている。
package devtools

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Glob は「/」区切りのパスの glob。* はフォルダの区切りをまたがず、** はまたぐ。
type Glob struct {
	Pattern string
	re      *regexp.Regexp
}

// NewGlob は glob を作る。
func NewGlob(pattern string) Glob {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case c == '*' && i+1 < len(pattern) && pattern[i+1] == '*':
			i++
			if i+1 < len(pattern) && pattern[i+1] == '/' {
				i++
				b.WriteString("(?:.*/)?") // **/ は 0 段以上のフォルダ
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '[':
			j := strings.IndexByte(pattern[i:], ']')
			if j < 0 {
				b.WriteString(regexp.QuoteMeta(string(c)))
				continue
			}
			class := pattern[i+1 : i+j]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			b.WriteString("[" + class + "]")
			i += j
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return Glob{Pattern: pattern, re: regexp.MustCompile(b.String())}
}

// Match はパス (「/」区切り、先頭の「/」なし) が glob に当たるか。
func (g Glob) Match(path string) bool { return g.re.MatchString(path) }

// FileList は data.files の書式 (1 行 1 つの glob、# は注釈、! で始まる行は除外) の一覧。
type FileList struct {
	Include []Glob
	Exclude []Glob
}

// ReadFileList は data.files を読む。
func ReadFileList(path string) (*FileList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	l := &FileList{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		switch {
		case s == "" || strings.HasPrefix(s, "#"):
		case strings.HasPrefix(s, "!"):
			l.Exclude = append(l.Exclude, NewGlob(s[1:]))
		default:
			l.Include = append(l.Include, NewGlob(s))
		}
	}
	return l, sc.Err()
}

// Match はパスが一覧に入るか (どれかの Include に当たり、どの Exclude にも当たらない)。
func (l *FileList) Match(path string) bool {
	in := false
	for _, g := range l.Include {
		if g.Match(path) {
			in = true
			break
		}
	}
	if !in {
		return false
	}
	for _, g := range l.Exclude {
		if g.Match(path) {
			return false
		}
	}
	return true
}

// Files は root の下で一覧に入るファイルを、root からの「/」区切りのパスで返す。
// .git や .data.git などの管理用フォルダと、skip に挙げたフォルダは見ない。
func (l *FileList) Files(root string, skip ...string) ([]string, error) {
	skipSet := map[string]bool{".git": true, ".data.git": true}
	for _, s := range skip {
		skipSet[s] = true
	}
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && skipSet[rel] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && l.Match(rel) {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
