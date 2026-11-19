// Package store 打开嵌入式 SQLite 并按序执行结构迁移。
// 运行时代码只认识当前结构：旧数据在启动时一次性迁移，热路径上没有兼容分支。
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// migrations 只增不改：新版本在末尾追加，下标 + 1 就是结构版本号。
var migrations = []string{
	// v1
	`
CREATE TABLE audit (
	seq       INTEGER PRIMARY KEY,
	ts        TEXT    NOT NULL,
	version   INTEGER NOT NULL,
	prev_hash TEXT    NOT NULL,
	hash      TEXT    NOT NULL,
	body      TEXT    NOT NULL
);
CREATE TRIGGER audit_no_update BEFORE UPDATE ON audit
BEGIN SELECT RAISE(ABORT, 'audit is append-only'); END;
CREATE TRIGGER audit_no_delete BEFORE DELETE ON audit
BEGIN SELECT RAISE(ABORT, 'audit is append-only'); END;

CREATE TABLE requests (
	id         TEXT PRIMARY KEY,
	created_at TEXT NOT NULL,
	state      TEXT NOT NULL,
	body       TEXT NOT NULL
);
CREATE INDEX requests_created ON requests (created_at DESC);

CREATE TABLE users (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE,
	role       TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE TABLE passkeys (
	id         BLOB PRIMARY KEY,
	user_id    TEXT NOT NULL REFERENCES users (id),
	name       TEXT NOT NULL,
	credential TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL REFERENCES users (id),
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE secrets (
	name        TEXT PRIMARY KEY,
	wrapped_key BLOB NOT NULL,
	ciphertext  BLOB NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE TABLE known_hosts (
	host     TEXT PRIMARY KEY,
	key      TEXT NOT NULL,
	added_at TEXT NOT NULL
);
CREATE TABLE settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`,
	// v2：在页面上管理的主机（配置文件里的主机不进数据库）
	`
CREATE TABLE hosts (
	name       TEXT PRIMARY KEY,
	body       TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
`,
	// v3：在页面上管理的 Snippet（正文是和文件相同格式的 YAML）
	`
CREATE TABLE snippets (
	name       TEXT PRIMARY KEY,
	body       TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
`,
	// v4：指标历史。res 是桶宽（秒）：60 是分钟级，900 是 15 分钟级。
	`
CREATE TABLE metrics (
	host TEXT    NOT NULL,
	res  INTEGER NOT NULL,
	ts   INTEGER NOT NULL,
	cpu  REAL, mem REAL, disk REAL, load REAL, swap REAL, rx REAL, tx REAL,
	PRIMARY KEY (host, res, ts)
) WITHOUT ROWID;
`,
	// v5：更多指标——缓存占比、5/15 分钟负载、磁盘读写速率、温度
	`
ALTER TABLE metrics ADD COLUMN cache  REAL NOT NULL DEFAULT 0;
ALTER TABLE metrics ADD COLUMN load5  REAL NOT NULL DEFAULT 0;
ALTER TABLE metrics ADD COLUMN load15 REAL NOT NULL DEFAULT 0;
ALTER TABLE metrics ADD COLUMN dr     REAL NOT NULL DEFAULT 0;
ALTER TABLE metrics ADD COLUMN dw     REAL NOT NULL DEFAULT 0;
ALTER TABLE metrics ADD COLUMN temp   REAL NOT NULL DEFAULT 0;
`,
	// v6：通知渠道（令牌、密码等机密不在这里，在凭据存储里）
	`
CREATE TABLE notify_channels (
	id         TEXT PRIMARY KEY,
	body       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
`,
	// v7：测速结果（从测速请求的输出里解析出来，每台主机只留最近的一些）
	`
CREATE TABLE speedtests (
	request_id TEXT    NOT NULL,
	host       TEXT    NOT NULL,
	ts         INTEGER NOT NULL,
	ping_ms    REAL    NOT NULL DEFAULT 0,
	down_bps   REAL    NOT NULL DEFAULT 0,
	up_bps     REAL    NOT NULL DEFAULT 0,
	size_mb    INTEGER NOT NULL DEFAULT 0,
	error      TEXT    NOT NULL DEFAULT '',
	PRIMARY KEY (request_id, host)
);
CREATE INDEX speedtests_host_ts ON speedtests (host, ts);
`,
	// v8：审计归档。检查点只追加；审计记录只有“已经归档、在最新检查点之前”的才能删除，其余照旧禁止。
	`
CREATE TABLE audit_checkpoints (
	upto       INTEGER PRIMARY KEY,
	hash       TEXT    NOT NULL,
	first      INTEGER NOT NULL,
	file       TEXT    NOT NULL,
	sha256     TEXT    NOT NULL,
	records    INTEGER NOT NULL,
	created_at TEXT    NOT NULL
);
CREATE TRIGGER audit_checkpoints_no_update BEFORE UPDATE ON audit_checkpoints
BEGIN SELECT RAISE(ABORT, 'audit checkpoints are append-only'); END;
CREATE TRIGGER audit_checkpoints_no_delete BEFORE DELETE ON audit_checkpoints
BEGIN SELECT RAISE(ABORT, 'audit checkpoints are append-only'); END;
DROP TRIGGER audit_no_delete;
CREATE TRIGGER audit_no_delete BEFORE DELETE ON audit
WHEN OLD.seq > COALESCE((SELECT MAX(upto) FROM audit_checkpoints), 0)
BEGIN SELECT RAISE(ABORT, 'audit is append-only (only archived records may be removed)'); END;
`,
	// v9：会话记下登录方式（passkey、oidc、dev），界面上显示“用什么登录的”。之前的会话都是 Passkey 登录的。
	`
ALTER TABLE sessions ADD COLUMN method TEXT NOT NULL DEFAULT 'passkey';
`,
	// v10：密码和动态码。密码只存 Argon2id 哈希；动态码的密钥存在凭据存储里（信封加密），这里只记引用和最后用过的时间步，防止同一个码用两次。
	`
CREATE TABLE user_passwords (
	user_id    TEXT PRIMARY KEY REFERENCES users (id),
	hash       TEXT    NOT NULL,
	initial    INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT    NOT NULL
);
CREATE TABLE user_totp (
	user_id    TEXT PRIMARY KEY REFERENCES users (id),
	secret_ref TEXT    NOT NULL,
	last_step  INTEGER NOT NULL DEFAULT 0,
	created_at TEXT    NOT NULL
);
`,
	// v11：测速报告的明细（每次连接的各段耗时、传输量和用时、出口和节点信息），JSON，带 "v" 版本号。之前的结果没有明细，留空。
	`
ALTER TABLE speedtests ADD COLUMN detail TEXT NOT NULL DEFAULT '';
`,
	// v12：通知渠道的语言代码和界面统一（web/src/locales/languages.json）：简体中文从 "zh" 改成 "zh-CN"。
	`
UPDATE notify_channels SET body = json_set(body, '$.lang', 'zh-CN') WHERE json_extract(body, '$.lang') = 'zh' OR json_extract(body, '$.lang') IS NULL;
`,
	// v13：测速记下用的方式（ookla、cloudflare）和 Ookla 的服务器名。之前的结果都是 Cloudflare 测的。
	`
ALTER TABLE speedtests ADD COLUMN method TEXT NOT NULL DEFAULT 'cloudflare';
ALTER TABLE speedtests ADD COLUMN server TEXT NOT NULL DEFAULT '';
`,
	// v14：测速记下线程（single、multi）和 IP 协议（4、6，空是自动）。之前的结果没有记，留空。
	`
ALTER TABLE speedtests ADD COLUMN threads TEXT NOT NULL DEFAULT '';
ALTER TABLE speedtests ADD COLUMN ip TEXT NOT NULL DEFAULT '';
`,
	// v15：月流量。按“主机 × 日期（面板时区）× 网卡”记字节数；另存每块网卡上次见到的累计计数器和主机开机时长，
	// 面板重启或断线之后接着算。配额设置是 JSON（带 "v" 版本号）；已经发过的流量告警按周期记下，每档只发一次。
	// 两个新的流量通知默认订阅，已有的通知渠道也加上。
	`
CREATE TABLE traffic_daily (
	host  TEXT NOT NULL,
	day   TEXT NOT NULL,
	iface TEXT NOT NULL,
	rx    REAL NOT NULL DEFAULT 0,
	tx    REAL NOT NULL DEFAULT 0,
	PRIMARY KEY (host, day, iface)
) WITHOUT ROWID;
CREATE TABLE traffic_counters (
	host  TEXT    NOT NULL,
	iface TEXT    NOT NULL,
	up    REAL    NOT NULL,
	ts    INTEGER NOT NULL,
	rx    REAL    NOT NULL,
	tx    REAL    NOT NULL,
	PRIMARY KEY (host, iface)
) WITHOUT ROWID;
CREATE TABLE traffic_quotas (
	host       TEXT PRIMARY KEY,
	body       TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE traffic_alerts (
	host   TEXT    NOT NULL,
	period TEXT    NOT NULL,
	level  TEXT    NOT NULL,
	ts     INTEGER NOT NULL,
	PRIMARY KEY (host, period, level)
) WITHOUT ROWID;
UPDATE notify_channels SET body = json_insert(json_insert(body, '$.events[#]', 'traffic.quota'), '$.events[#]', 'traffic.forecast')
WHERE json_type(body, '$.events') = 'array';
`,
}

// SchemaVersion 是当前代码认识的结构版本。
func SchemaVersion() int { return len(migrations) }

// Open 打开（必要时创建）数据目录里的数据库，并迁移到当前结构。
func Open(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	dsn := "file:" + filepath.ToSlash(filepath.Join(dataDir, "hostower.db")) + "?" + q.Encode()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // 单写者；单节点足够
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	var have int
	if err := db.QueryRow("PRAGMA user_version").Scan(&have); err != nil {
		return err
	}
	if have > len(migrations) {
		return fmt.Errorf("数据结构版本 %d 比本程序认识的 %d 新：请用更新的版本启动，或从快照恢复", have, len(migrations))
	}
	for v := have; v < len(migrations); v++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("迁移到结构版本 %d 失败：%w", v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
