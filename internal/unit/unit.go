// Package unit は抽出・翻訳・差し戻しで共通に使う翻訳単位 (TranslationUnit) と
// その JSONL 入出力を定義する。
package unit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Status は翻訳単位の状態。
type Status string

const (
	StatusNew      Status = "new"      // 未翻訳
	StatusMachine  Status = "machine"  // 機械翻訳済み
	StatusReviewed Status = "reviewed" // 人が確認・修正済み
	StatusLocked   Status = "locked"   // 再翻訳で上書きしない
)

// Done は訳文が確定している (再翻訳不要な) 状態かどうか。
func (s Status) Done() bool {
	return s == StatusMachine || s == StatusReviewed || s == StatusLocked
}

// Context は訳すときの手がかり。
type Context struct {
	Speaker string `json:"speaker,omitempty"` // 話者
	Scene   string `json:"scene,omitempty"`   // シーン/事件など。同じ値の行は順番に訳し、前の行を履歴として渡す
	Kind    string `json:"kind,omitempty"`    // dialogue, ui, item など
	Note    string `json:"note,omitempty"`    // 状況説明など自由記述
}

// Constraints は訳文に課す制約。
type Constraints struct {
	MaxLen int `json:"max_len,omitempty"` // 訳文の最大文字数 (rune 数)。0 なら無制限
}

// Unit は翻訳単位 1 件。
type Unit struct {
	ID          string      `json:"id"`
	Source      string      `json:"source"`
	Target      string      `json:"target,omitempty"`
	Context     Context     `json:"context"`
	Constraints Constraints `json:"constraints"`
	Status      Status      `json:"status,omitempty"`
	Hash        string      `json:"hash,omitempty"`     // 原文のハッシュ。原文が変わったら再翻訳対象
	Warnings    []string    `json:"warnings,omitempty"` // 検証で見つかった問題
}

// SourceHash は原文のハッシュ (16 進 32 文字)。
func SourceHash(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:16])
}

// ReadJSONL は JSONL ファイルを読む。空行は無視する。
func ReadJSONL(path string) ([]Unit, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var units []Unit
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	line := 0
	for sc.Scan() {
		line++
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var u Unit
		if err := json.Unmarshal(b, &u); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if u.ID == "" {
			return nil, fmt.Errorf("%s:%d: id が空です", path, line)
		}
		units = append(units, u)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return units, nil
}

// WriteJSONL は units を一時ファイルに書いてから置き換える (途中で落ちても元ファイルが壊れない)。
func WriteJSONL(path string, units []Unit) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	w := bufio.NewWriter(tmp)
	enc := newEncoder(w)
	for i := range units {
		if err := enc.Encode(&units[i]); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return err
		}
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { // CreateTemp は 0600 で作るため
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Appender は JSONL に 1 件ずつ追記する。複数 goroutine から使える。
// 翻訳中の途中経過をここに書き、中断しても再開できるようにする。
type Appender struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// OpenAppender は path を追記モードで開く。
func OpenAppender(path string) (*Appender, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Appender{f: f, enc: newEncoder(f)}, nil
}

// Append は 1 件書き込む。
func (a *Appender) Append(u Unit) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.enc.Encode(&u)
}

// Close はファイルを閉じる。
func (a *Appender) Close() error { return a.f.Close() }

func newEncoder(w interface{ Write([]byte) (int, error) }) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // <b> などのタグを < にしない
	return enc
}
