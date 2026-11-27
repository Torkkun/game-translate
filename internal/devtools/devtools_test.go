package devtools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlob(t *testing.T) {
	cases := []struct {
		pat, path string
		want      bool
	}{
		{"games/**", "games/x/ja.jsonl", true},
		{"games/**", "games", false},
		{"cmd/gtl/game_*.go", "cmd/gtl/game_x.go", true},
		{"cmd/gtl/game_*.go", "cmd/gtl/sub/game_x.go", false},
		{"**/bin/**", "games/x/hook/bin/a.dll", true},
		{"**/bin/**", "bin/a", true},
		{"**/*.patchwad", "games/x/mod/a.patchwad", true},
		{"docs/private/**", "docs/private/a/b.md", true},
		{"[!t]*.jsonl", "testdata/a.jsonl", false},
	}
	for _, c := range cases {
		if got := NewGlob(c.pat).Match(c.path); got != c.want {
			t.Errorf("%s ~ %s = %v, want %v", c.pat, c.path, got, c.want)
		}
	}
}

func TestFileList(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"games/x/ja.jsonl", "games/x/mod/a.patchwad", "games/x/hook/bin/a.dll", "cmd/gtl/main.go", "cmd/gtl/game_x.go"} {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	list := filepath.Join(dir, "data.files")
	if err := os.WriteFile(list, []byte("# 注釈\ngames/**\ncmd/gtl/game_*.go\n!**/bin/**\n!**/*.patchwad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := ReadFileList(list)
	if err != nil {
		t.Fatal(err)
	}
	got, err := l.Files(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd/gtl/game_x.go", "games/x/ja.jsonl"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("%v", got)
	}
}

func TestClassify(t *testing.T) {
	for p, bad := range map[string]bool{
		"games/x/game.yaml":        true,
		"internal/adapter/x/x.go":  true,
		"cmd/gtl/game_x.go":        true,
		"magefiles/game_x.go":      true,
		"docs/private/ENV.md":      true,
		"a/b.jsonl":                true,
		"testdata/sample.jsonl":    false,
		"x/out.wad.pc":             true,
		"tools/a.dll":              true,
		"cmd/gtl/main.go":          false,
		"internal/translate/a.go":  false,
		"magefiles/magefile.go":    false,
		"docs/ENGINES.md":          false,
		".github/README.md":        true,
		".github/workflows/ci.yml": false,
	} {
		if got := Classify(p) != ""; got != bad {
			t.Errorf("%s: %v, want %v (%s)", p, got, bad, Classify(p))
		}
	}
}
