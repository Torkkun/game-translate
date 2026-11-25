package install

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallUninstall(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	game := filepath.Join(root, "game")
	backup := filepath.Join(root, "backup")
	keep := filepath.Join(root, "keep")

	write(t, filepath.Join(dist, "winhttp.dll"), "doorstop")
	write(t, filepath.Join(dist, "Loader/plugins/X/x.dll"), "plugin")
	write(t, filepath.Join(dist, "doorstop_config.ini"), "new config")
	write(t, filepath.Join(game, "Game.exe"), "exe")
	write(t, filepath.Join(game, "doorstop_config.ini"), "user's old config") // 既存で中身が違う

	if _, err := Install("g", dist, game, "Missing.exe", backup); err == nil {
		t.Fatal("exe が無いフォルダには入れないはず")
	}
	res, err := Install("g", dist, game, "Game.exe", backup)
	if err != nil {
		t.Fatal(err)
	}
	if res.Copied != 3 || res.BackedUp != 1 {
		t.Fatalf("3 件コピー・1 件退避のはず: %+v", res)
	}
	if read(t, filepath.Join(game, "doorstop_config.ini")) != "new config" {
		t.Fatal("上書きされていない")
	}
	if _, err := Install("g", dist, game, "Game.exe", backup); err == nil {
		t.Fatal("二重インストールは拒否するはず")
	}

	// ゲームを動かすと Mod のフォルダにログなどができる
	write(t, filepath.Join(game, "Loader/plugins/X/untranslated.tsv"), "NGUI\tHELLO\n")
	write(t, filepath.Join(game, "Loader/LogOutput.log"), "log")

	res, err = Uninstall(game, keep, []string{"untranslated.tsv"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Restored != 1 || len(res.Preserved) != 1 {
		t.Fatalf("1 件復元・1 件退避のはず: %+v", res)
	}
	if read(t, filepath.Join(game, "doorstop_config.ini")) != "user's old config" {
		t.Fatal("元のファイルが戻っていない")
	}
	for _, p := range []string{"winhttp.dll", "Loader", ManifestName} {
		if _, err := os.Stat(filepath.Join(game, p)); err == nil {
			t.Fatalf("%s が残っている", p)
		}
	}
	if read(t, filepath.Join(game, "Game.exe")) != "exe" {
		t.Fatal("ゲーム本体のファイルに触ってはいけない")
	}
	if read(t, res.Preserved[0]) != "NGUI\tHELLO\n" {
		t.Fatal("記録類が退避されていない")
	}
}
