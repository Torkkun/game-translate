//go:build mage

// 開発用のタスク ([mage](https://magefile.org/))。
//
//	./scripts/mage.sh              タスクの一覧 (Go が無い PC 向け: コンテナで組み立てて手元で動かす)
//	go run mage.go <タスク>         Go がある PC では直接
//
// 普段使うツール (golangci-lint・shellcheck・actionlint・gitleaks) は、Dev Container の中なら直接、
// 無ければその都度 Docker のコンテナで動かす (ホストには Docker・git・gh だけあればよい)。
// 組み立てのときだけ使うもの (.NET・MinGW・Python) はいつもコンテナで動かす。
// ゲーム固有のタスク (Mod の組み立て) は magefiles/game_*.go に書く (データ用リポジトリで管理)。
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"

	"github.com/Torkkun/game-translate/internal/devtools"
)

// 使うコンテナの版 (CI と同じ)
const (
	goImage         = "mcr.microsoft.com/devcontainers/go:2-1.27-trixie"
	golangciImage   = "golangci/golangci-lint:v2.15.0"
	shellcheckImage = "koalaman/shellcheck:v0.11.0"
	ruffImage       = "ghcr.io/astral-sh/ruff:0.17.6"
	dotnetImage     = "mcr.microsoft.com/dotnet/sdk:8.0"
	actionlintImage = "rhysd/actionlint:1.7.12"
	gitleaksImage   = "zricethezav/gitleaks:v8.30.1"
)

// dataGitDir はデータ用リポジトリの git の管理情報の場所。
const dataGitDir = ".data.git"

func root() string {
	d, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return d
}

// docker は docker run --rm に、作業フォルダのマウントと (Linux なら) 自分のユーザーを付けて実行する。
// mountAt はコンテナ内での作業フォルダの場所 (空なら手元と同じパス)。
func docker(mountAt string, user bool, args ...string) error {
	r := root()
	if mountAt == "" {
		mountAt = r
	}
	a := []string{"run", "--rm", "-v", r + ":" + mountAt, "-w", mountAt}
	if user && runtime.GOOS == "linux" {
		a = append(a, "-u", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()))
	}
	return sh.RunV("docker", append(a, args...)...)
}

// goInDocker は Go のコンテナで go コマンドを実行する (キャッシュは .cache/)。
func goInDocker(args ...string) error {
	r := root()
	a := []string{"-e", "HOME=/tmp", "-e", "GOCACHE=" + r + "/.cache/go-build", "-e", "GOMODCACHE=" + r + "/.cache/go-mod",
		"-e", "GOLANGCI_LINT_CACHE=" + r + "/.cache/golangci"}
	return docker("", true, append(a, args...)...)
}

// local は、ツールが手元 (Dev Container の中など) にあるか。無ければその都度コンテナで動かす。
func local(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ---------------------------------------------------------------- check

// 公開リポジトリに入れてはいけないもの (ゲーム由来のデータ・訳文・ゲーム固有の拡張・大きなファイル・個人の環境のパス) が無いか確かめる。
func Check() error {
	problems, err := devtools.CheckPublic(root())
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Println("NG:", p)
		}
		return fmt.Errorf("公開してはいけないものが %d 件あります。.gitignore と scripts/data.files を確認してください", len(problems))
	}
	fmt.Println("OK: 公開して問題になりそうなファイルはありません")
	return nil
}

// ---------------------------------------------------------------- lint

// CI と同じ確認 (と、手元にあればゲーム固有の拡張の確認)。
type Lint mg.Namespace

// すべての確認をする。変更したら必ず通す。
func (Lint) All() {
	mg.SerialDeps(Lint.Guard, Lint.Go, Lint.Shell, Lint.Python, Lint.CSharp, Lint.Actions)
}

// 公開して問題ないか (Check と、gitleaks での秘密情報の確認)。
func (Lint) Guard() error {
	fmt.Println("== 公開して問題ないか")
	if err := Check(); err != nil {
		return err
	}
	if local("gitleaks") {
		return sh.RunV("gitleaks", "git", ".", "--no-banner", "--redact", "--log-opts=HEAD")
	}
	return docker("/repo", false, gitleaksImage, "git", "/repo", "--no-banner", "--redact", "--log-opts=HEAD")
}

// gofmt・go vet・golangci-lint・go test。
func (Lint) Go() error {
	fmt.Println("== Go")
	goRun := func(args ...string) error {
		if local("go") {
			return sh.RunV("go", args...)
		}
		return goInDocker(append([]string{goImage, "go"}, args...)...)
	}
	var out string
	var err error
	if local("gofmt") {
		out, err = sh.Output("gofmt", "-l", "cmd", "internal", "magefiles")
	} else {
		var b []byte
		b, err = exec.Command("docker", "run", "--rm", "-v", root()+":"+root(), "-w", root(), goImage, "gofmt", "-l", "cmd", "internal", "magefiles").Output()
		out = string(b)
	}
	if err != nil {
		return fmt.Errorf("gofmt: %w", err)
	}
	if s := strings.TrimSpace(out); s != "" {
		return fmt.Errorf("gofmt で整形されていないファイル:\n%s", s)
	}
	if err := goRun("vet", "-tags", "mage", "./..."); err != nil {
		return err
	}
	if local("golangci-lint") {
		err = sh.RunV("golangci-lint", "run", "--build-tags", "mage", "./...")
	} else {
		err = goInDocker(golangciImage, "golangci-lint", "run", "--build-tags", "mage", "./...")
	}
	if err != nil {
		return err
	}
	return goRun("test", "./...")
}

// 残っているシェルスクリプトの shellcheck。
func (Lint) Shell() error {
	fmt.Println("== Shell")
	files := globs("scripts/*.sh", "games/*/hook/build.sh")
	if len(files) == 0 {
		return nil
	}
	if local("shellcheck") {
		return sh.RunV("shellcheck", files...)
	}
	return docker("/mnt/r", false, append([]string{shellcheckImage}, files...)...)
}

// ゲーム固有の補助ツール (あれば) の ruff。普段の開発では使わないので、いつもコンテナで動かす。
func (Lint) Python() error {
	fmt.Println("== Python")
	var files []string
	_ = filepath.WalkDir("games", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".py") && !strings.Contains(p, "__pycache__") {
			files = append(files, filepath.ToSlash(p))
		}
		return nil
	})
	if len(files) == 0 {
		return nil
	}
	if err := docker("/w", true, append([]string{ruffImage, "check"}, files...)...); err != nil {
		return err
	}
	return docker("/w", true, append([]string{ruffImage, "format", "--check"}, files...)...)
}

// プラグイン (あれば) の整形の確認。.NET は組み立てのときにしか使わないので、いつもコンテナで動かす。
func (Lint) CSharp() error {
	fmt.Println("== C#")
	for _, p := range globs("plugins/*/") {
		r := root()
		err := sh.RunV("docker", "run", "--rm", "-e", "DOTNET_CLI_TELEMETRY_OPTOUT=1", "-e", "DOTNET_NOLOGO=1",
			"-v", r+":"+r, "-w", filepath.Join(r, p), dotnetImage, "dotnet", "format", "whitespace", "--folder", "--verify-no-changes")
		if err != nil {
			return err
		}
	}
	return nil
}

// GitHub Actions のワークフローの actionlint。
func (Lint) Actions() error {
	fmt.Println("== GitHub Actions")
	if local("actionlint") {
		return sh.RunV("actionlint", "-color")
	}
	return docker("/repo", false, actionlintImage, "-color")
}

func globs(patterns ...string) []string {
	var out []string
	for _, p := range patterns {
		m, _ := filepath.Glob(p)
		for _, f := range m {
			out = append(out, filepath.ToSlash(f))
		}
	}
	return out
}

// ---------------------------------------------------------------- data

// データ用リポジトリ (ゲーム由来のデータ・訳文・ゲーム固有の拡張の保存先。非公開で作る) の操作。
// 作業フォルダはこのリポジトリと同じで、git の管理情報だけを .data.git に分ける。
// 追跡するのは scripts/data.files に書いたものだけ。
type Data mg.Namespace

func dataGit(args ...string) error {
	return sh.RunV("git", append([]string{"--git-dir=" + dataGitDir, "--work-tree=."}, args...)...)
}

func dataGitOut(args ...string) (string, error) {
	return sh.Output("git", append([]string{"--git-dir=" + dataGitDir, "--work-tree=."}, args...)...)
}

// 一覧以外のファイルは status に出さない (このリポジトリのファイルは追跡しない)
func dataExclude() error {
	if err := os.MkdirAll(filepath.Join(dataGitDir, "info"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataGitDir, "info", "exclude"), []byte("*\n"), 0o644)
}

// データ用リポジトリを用意する (初回・別の PC で)。リモートが空なら作り、あれば取得する。
func (Data) Init(url string) error {
	if _, err := os.Stat(dataGitDir); err == nil {
		return fmt.Errorf("%s はすでにあります", dataGitDir)
	}
	if err := sh.Run("git", "init", "-q", "--bare", dataGitDir); err != nil {
		return err
	}
	_ = sh.Run("git", "--git-dir="+dataGitDir, "config", "core.bare", "false")
	_ = sh.Run("git", "--git-dir="+dataGitDir, "config", "core.worktree", "..")
	for _, k := range []string{"user.name", "user.email"} { // このリポジトリと同じ作者情報
		if v, err := sh.Output("git", "config", k); err == nil && v != "" {
			_ = sh.Run("git", "--git-dir="+dataGitDir, "config", k, v)
		}
	}
	if err := dataExclude(); err != nil {
		return err
	}
	if err := dataGit("remote", "add", "origin", url); err != nil {
		return err
	}
	if err := exec.Command("git", "--git-dir="+dataGitDir, "ls-remote", "--exit-code", "origin", "HEAD").Run(); err == nil {
		if err := dataGit("fetch", "-q", "origin"); err != nil {
			return err
		}
		return dataGit("checkout", "-q", "-B", "main", "origin/main")
	}
	fmt.Println("空のリポジトリとして作りました。mage data:save で最初の保存をします")
	return dataGit("checkout", "-q", "-b", "main")
}

// scripts/data.files に当たるファイルをすべて commit して push する。
func (Data) Save(message string) error {
	if _, err := os.Stat(dataGitDir); err != nil {
		return fmt.Errorf("データ用リポジトリがありません。先に mage data:init <URL>")
	}
	if err := dataExclude(); err != nil {
		return err
	}
	l, err := devtools.ReadFileList("scripts/data.files")
	if err != nil {
		return err
	}
	files, err := l.Files(".", "models", "work", "dist", ".cache", "bin")
	if err != nil {
		return err
	}
	// .gitignore にも当たるので -f で足す (引数が長くなりすぎないよう分ける)
	for i := 0; i < len(files); i += 200 {
		j := min(i+200, len(files))
		if err := dataGit(append([]string{"add", "-f", "--"}, files[i:j]...)...); err != nil {
			return err
		}
	}
	if err := dataGit("add", "-u"); err != nil { // 消したファイル
		return err
	}
	if exec.Command("git", "--git-dir="+dataGitDir, "--work-tree=.", "diff", "--cached", "--quiet").Run() == nil {
		fmt.Println("変更はありません")
	} else {
		if message == "" {
			message = "訳文・データを保存"
		}
		if err := dataGit("commit", "-q", "-m", message); err != nil {
			return err
		}
		fmt.Println("commit しました:", message)
	}
	if err := dataGit("push", "-q", "-u", "origin", "main"); err != nil {
		return err
	}
	fmt.Println("push しました")
	return nil
}

// データ用リポジトリの変更を表示する。
func (Data) Status() error { return dataGit("status", "--short") }

// データ用リポジトリで git のコマンドを実行する (例: mage data:git "log --oneline")。
func (Data) Git(args string) error { return dataGit(strings.Fields(args)...) }

// ---------------------------------------------------------------- release

//	Mod と訳文をデータ用リポジトリに保存する (ゲーム・版ごと)。
//
// 訳文などを commit してタグ <game>-<版> を付け、dist/<game> (Mod 一式) と翻訳メモリの zip を Release <game>-<版> に添付する。
// 先に Mod を作っておく (mage package:<game> など)。ゲーム由来のファイルを含むので、このリポジトリには上げない。
func Release(game, version, note string) error {
	tag := game + "-" + version
	if _, err := os.Stat(dataGitDir); err != nil {
		return fmt.Errorf("データ用リポジトリがありません。先に mage data:init <URL>")
	}
	if _, err := os.Stat(filepath.Join("games", game)); err != nil {
		return fmt.Errorf("games/%s がありません", game)
	}
	src := ""
	for _, d := range []string{filepath.Join("dist", game), filepath.Join("games", game, "mod")} {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			src = d
			break
		}
	}
	if src == "" {
		return fmt.Errorf("dist/%s に Mod がありません。先に作ってください", game)
	}
	url, err := dataGitOut("remote", "get-url", "origin")
	if err != nil {
		return err
	}
	repo := regexp.MustCompile(`^(https://github\.com/|git@github\.com:)|\.git$`).ReplaceAllString(url, "")
	if exec.Command("git", "--git-dir="+dataGitDir, "rev-parse", "-q", "--verify", "refs/tags/"+tag).Run() == nil {
		return fmt.Errorf("タグ %s はすでにあります。版を上げてください", tag)
	}
	msg := game + " " + version
	if note != "" {
		msg += ": " + note
	}
	// 1. 訳文・データ
	if err := (Data{}).Save(msg); err != nil {
		return err
	}
	if err := dataGit("tag", "-a", tag, "-m", msg); err != nil {
		return err
	}
	if err := dataGit("push", "-q", "origin", tag); err != nil {
		return err
	}
	// 2. Mod 一式 + 翻訳メモリ
	zipPath := filepath.Join("work", "releases", tag+".zip")
	extra := []string{}
	if _, err := os.Stat(filepath.Join("work", game, "tm.sqlite")); err == nil {
		extra = append(extra, filepath.Join("work", game, "tm.sqlite"))
	}
	size, err := writeZip(zipPath, src, extra)
	if err != nil {
		return err
	}
	if size >= 2000<<20 {
		return fmt.Errorf("zip が %dMB あり、Release に添付できる上限 (2GB) を超えます: %s", size>>20, zipPath)
	}
	body := fmt.Sprintf("%s\n\n- Mod: %s\n- 訳文・データ: タグ %s\n", orDefault(note, msg), filepath.ToSlash(src), tag)
	if err := sh.RunV("gh", "release", "create", tag, zipPath, "--repo", repo, "--title", game+" "+version, "--notes", body); err != nil {
		return err
	}
	fmt.Printf("保存しました: %s の Release %s (%dMB)\n", repo, tag, size>>20)
	return nil
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// writeZip は dir の中身を mod/ の下に、extra のファイルを最上位に入れた zip を作り、大きさを返す。
func writeZip(path, dir string, extra []string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	zw := zip.NewWriter(f)
	add := func(src, name string) error {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		w, err := zw.CreateHeader(&zip.FileHeader{Name: filepath.ToSlash(name), Method: zip.Deflate})
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		return err
	}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		return add(p, filepath.Join("mod", rel))
	})
	for _, e := range extra {
		if err == nil {
			err = add(e, filepath.Base(e))
		}
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// ---------------------------------------------------------------- package

// ゲームごとの Mod の組み立て。タスクは magefiles/game_*.go (ゲーム固有の拡張) が足す。
type Package mg.Namespace
