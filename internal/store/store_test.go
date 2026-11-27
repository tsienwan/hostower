package store

import (
	"database/sql"
	"fmt"
	"testing"
)

// v19：默认监听端口换了之后，已经在用的面板保持原来的端口；新装的和自己设过的不动。
func TestKeepOldListen(t *testing.T) {
	listen := func(db *sql.DB) string {
		var v string
		db.QueryRow(`SELECT value FROM settings WHERE key = 'panel.listen'`).Scan(&v)
		return v
	}
	// 回到 v18 再打开一次，让 v19 重新跑。
	reopen := func(dir string, db *sql.DB, prepare string) *sql.DB {
		if _, err := db.Exec(prepare); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", 18)); err != nil {
			t.Fatal(err)
		}
		db.Close()
		db, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	open := func() (string, *sql.DB) {
		dir := t.TempDir()
		db, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		return dir, db
	}
	const user = `INSERT INTO users (id, name, role, created_at) VALUES ('u1', 'owner', 'owner', '2026-01-01T00:00:00Z')`

	dir, db := open()
	if got := listen(db); got != "" {
		t.Fatalf("新装的面板不应该被固定端口，现在是 %q", got)
	}
	db = reopen(dir, db, user)
	if got := listen(db); got != "127.0.0.1:18443" {
		t.Fatalf("已经在用的面板应该保持原来的默认端口，现在是 %q", got)
	}
	db.Close()

	dir, db = open()
	db = reopen(dir, db, user+`; INSERT INTO settings (key, value) VALUES ('panel.listen', '0.0.0.0:9000')`)
	if got := listen(db); got != "0.0.0.0:9000" {
		t.Fatalf("自己设过监听地址的不应该被改，现在是 %q", got)
	}
	db.Close()
}
