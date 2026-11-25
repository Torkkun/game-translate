// Package install は dist/<game> の Mod 一式をゲームフォルダに入れる・外す。
//
//   - 入れたファイルの一覧を、ゲームフォルダの gtl-install.json に記録する
//     (ゲームをアンインストールすると一緒に消えるので、再インストール後はそのまま入れ直せる)
//   - すでにあって中身が違うファイルは、上書き前に backupDir に退避する
//   - 外すときは、記録したファイルを消し、Mod が新しく作ったフォルダ (Mod ローダーのフォルダなど) は中身ごと消す。
//     退避したファイルがあれば元に戻す
package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ManifestName はゲームフォルダに置く記録ファイルの名前。
const ManifestName = "gtl-install.json"

// Manifest はインストール記録。
type Manifest struct {
	Game        string            `json:"game"`
	InstalledAt time.Time         `json:"installed_at"`
	Files       map[string]string `json:"files"`        // ゲームフォルダからの相対パス → sha256
	CreatedDirs []string          `json:"created_dirs"` // Mod が新しく作った最上位のフォルダ
	BackupDir   string            `json:"backup_dir,omitempty"`
	BackedUp    []string          `json:"backed_up,omitempty"` // 上書き前に退避したファイル
}

// Result は処理結果の集計。
type Result struct {
	Copied, Unchanged, BackedUp, Removed, Restored int
	Preserved                                      []string // 外す前に退避した、Mod が作った記録類
}

// Install は distDir の中身を gameDir に入れる。exe はゲームフォルダの確認用 (空なら確認しない)。
func Install(game, distDir, gameDir, exe, backupDir string) (*Result, error) {
	if err := checkGameDir(gameDir, exe); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(gameDir, ManifestName)); err == nil {
		return nil, fmt.Errorf("すでにインストールされています (%s)。先に uninstall してください", ManifestName)
	}
	files, err := listFiles(distDir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w (先に gtl build で Mod を作ってください)", distDir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s が空です", distDir)
	}

	m := &Manifest{Game: game, InstalledAt: time.Now(), Files: map[string]string{}}
	res := &Result{}

	// 新しく作ることになる最上位のフォルダ
	tops := map[string]bool{}
	for _, f := range files {
		if i := strings.IndexByte(f, '/'); i > 0 {
			tops[f[:i]] = true
		}
	}
	for d := range tops {
		if _, err := os.Stat(filepath.Join(gameDir, d)); errors.Is(err, fs.ErrNotExist) {
			m.CreatedDirs = append(m.CreatedDirs, d)
		}
	}
	sort.Strings(m.CreatedDirs)

	stamp := time.Now().Format("20060102-150405")
	for _, f := range files {
		src := filepath.Join(distDir, filepath.FromSlash(f))
		dst := filepath.Join(gameDir, filepath.FromSlash(f))
		sum, err := fileHash(src)
		if err != nil {
			return res, err
		}
		if old, err := fileHash(dst); err == nil {
			if old == sum {
				m.Files[f] = sum
				res.Unchanged++
				continue
			}
			// 中身の違う既存ファイルは退避してから上書きする
			if m.BackupDir == "" {
				m.BackupDir = filepath.Join(backupDir, stamp)
			}
			if err := copyFile(dst, filepath.Join(m.BackupDir, filepath.FromSlash(f))); err != nil {
				return res, fmt.Errorf("%s の退避: %w", f, err)
			}
			m.BackedUp = append(m.BackedUp, f)
			res.BackedUp++
		}
		if err := copyFile(src, dst); err != nil {
			return res, fmt.Errorf("%s のコピー: %w (ゲームを起動中なら終了してください)", f, err)
		}
		m.Files[f] = sum
		res.Copied++
	}
	return res, writeManifest(gameDir, m)
}

// Uninstall は gameDir から Mod を外す。preserveDir があれば、Mod が作った記録類
// (未翻訳文字列の記録など、keep に一致するファイル) をそこへ退避してから消す。
func Uninstall(gameDir, preserveDir string, keep []string) (*Result, error) {
	m, err := ReadManifest(gameDir)
	if err != nil {
		return nil, err
	}
	res := &Result{}

	// Mod が作ったフォルダの中で、残しておきたい記録類を退避する
	if preserveDir != "" {
		stamp := time.Now().Format("20060102-150405")
		for _, d := range m.CreatedDirs {
			// 退避は記録の保存だけなので、失敗しても続ける
			_ = filepath.WalkDir(filepath.Join(gameDir, d), func(p string, e fs.DirEntry, err error) error {
				if err != nil || e.IsDir() {
					return nil
				}
				for _, k := range keep {
					if e.Name() == k {
						rel, _ := filepath.Rel(gameDir, p)
						to := filepath.Join(preserveDir, stamp, rel)
						if copyFile(p, to) == nil {
							res.Preserved = append(res.Preserved, to)
						}
					}
				}
				return nil
			})
		}
	}

	for f := range m.Files {
		p := filepath.Join(gameDir, filepath.FromSlash(f))
		if err := os.Remove(p); err == nil {
			res.Removed++
		} else if !errors.Is(err, fs.ErrNotExist) {
			return res, fmt.Errorf("%s を消せません: %w (ゲームを起動中なら終了してください)", f, err)
		}
	}
	for _, d := range m.CreatedDirs {
		if err := os.RemoveAll(filepath.Join(gameDir, d)); err != nil {
			return res, err
		}
	}
	for _, f := range m.BackedUp {
		if err := copyFile(filepath.Join(m.BackupDir, filepath.FromSlash(f)), filepath.Join(gameDir, filepath.FromSlash(f))); err != nil {
			return res, fmt.Errorf("%s の復元: %w", f, err)
		}
		res.Restored++
	}
	return res, os.Remove(filepath.Join(gameDir, ManifestName))
}

// ReadManifest はゲームフォルダのインストール記録を読む。
func ReadManifest(gameDir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(gameDir, ManifestName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("インストール記録 (%s) がありません。gtl install で入れた Mod ではないか、未インストールです", ManifestName)
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestName, err)
	}
	return &m, nil
}

func writeManifest(gameDir string, m *Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(gameDir, ManifestName), append(b, '\n'), 0o644)
}

func checkGameDir(gameDir, exe string) error {
	st, err := os.Stat(gameDir)
	if err != nil {
		return fmt.Errorf("ゲームフォルダが見つかりません: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("%s はフォルダではありません", gameDir)
	}
	if exe != "" {
		if _, err := os.Stat(filepath.Join(gameDir, exe)); err != nil {
			return fmt.Errorf("%s に %s がありません。ゲームフォルダが違うようです", gameDir, exe)
		}
	}
	return nil
}

// listFiles は dir 以下のファイルを「/ 区切りの相対パス」で返す。
func listFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

func fileHash(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
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
