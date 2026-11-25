package gameconf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalOverride(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(dir)
	os.MkdirAll("games/x", 0o755)
	os.WriteFile(filepath.Join("games/x/game.yaml"), []byte("title: X\ninstall_dir: /mnt/c/Steam/X\nexe: x.exe\n"), 0o644)
	os.WriteFile(filepath.Join("games/x/game.local.yaml"), []byte("install_dir: /mnt/d/Games/X\n"), 0o644)
	c, err := Load("x")
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "X" || c.Exe != "x.exe" || c.InstallDir != "/mnt/d/Games/X" {
		t.Fatalf("%+v", c)
	}
}

func TestWindowsPath(t *testing.T) {
	for in, want := range map[string]string{
		"/mnt/d/Games/a b": "D:/Games/a b",
		"/mnt/c":           "C:/",
		"/home/x":          "/home/x",
	} {
		if got := WindowsPath(in); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}
