// Package tm は翻訳メモリ (原文→訳文のキャッシュ) を SQLite に保存する。
// 同じ台詞が何度も出るゲームでの重複翻訳を避け、gtl serve でも即答に使う。
package tm

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // cgo 不要の SQLite ドライバ
)

// TM は翻訳メモリ。
type TM struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS tm (
	key        TEXT PRIMARY KEY,
	source     TEXT NOT NULL,
	target     TEXT NOT NULL,
	lang       TEXT NOT NULL,
	model      TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);`

// Open は path の翻訳メモリを開く (無ければ作る)。
func Open(path string) (*TM, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // 書き込みの競合を避ける
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &TM{db: db}, nil
}

// Close は DB を閉じる。
func (t *TM) Close() error { return t.db.Close() }

// Key は原文・話者・訳先言語・文体から引くためのキーを作る。
// 同じ英文でも話者が違えば口調 (性別・一人称) が変わるので、話者もキーに含める。
func Key(source, speaker, targetLang, form string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + speaker + "\x00" + targetLang + "\x00" + form))
	return hex.EncodeToString(sum[:])
}

// Get は訳文を引く。見つからなければ ok=false。
func (t *TM) Get(key string) (target string, ok bool, err error) {
	err = t.db.QueryRow(`SELECT target FROM tm WHERE key = ?`, key).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return target, true, nil
}

// Put は訳文を保存する (同じキーがあれば上書き)。
func (t *TM) Put(key, source, target, lang, model string) error {
	_, err := t.db.Exec(`
INSERT INTO tm (key, source, target, lang, model, updated_at) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET target = excluded.target, model = excluded.model, updated_at = excluded.updated_at`,
		key, source, target, lang, model, time.Now().Unix())
	return err
}

// Count は保存件数を返す。
func (t *TM) Count() (int, error) {
	var n int
	err := t.db.QueryRow(`SELECT COUNT(*) FROM tm`).Scan(&n)
	return n, err
}
