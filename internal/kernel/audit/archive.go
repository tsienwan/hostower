package audit

// 归档：数据库里的审计记录太多时，把旧的一段写进带摘要的归档文件、记一个检查点，再从数据库里删掉。
//
// 这是审计记录唯一的删除路径，而且删之前一定已经：
//   1. 重新校验过这一段的哈希链（断了就不归档）；
//   2. 把这一段原样写进 audit-archive/audit-<起>-<止>.jsonl.gz（先写临时文件、落盘、再改名）；
//   3. 在链里追加一条 audit.archived 记录，写进文件的摘要和最后一条的哈希（写不进去就不删）；
//   4. 在只追加的 audit_checkpoints 表里记下检查点。
// 数据库的触发器只允许删除最新检查点之前的记录。校验从最新检查点接着往后算；
// 离线校验（hostower audit verify）还会逐个校验归档文件的摘要和首尾相接。

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Checkpoint 是一次归档留下的记号。
type Checkpoint struct {
	Upto      int64  `json:"upto"`    // 归档到哪一条（含）
	Hash      string `json:"hash"`    // 那一条的哈希：数据库里剩下的第一条的 prev_hash 必须等于它
	First     int64  `json:"first"`   // 这一段从哪一条开始
	File      string `json:"file"`    // 归档文件名（在归档目录里）
	SHA256    string `json:"sha256"`  // 归档文件的摘要
	Records   int64  `json:"records"` // 这一段有多少条
	CreatedAt string `json:"createdAt"`
}

// Policy 是自动归档的条件：数据库里的记录超过 MaxRecords 条时，把早于 KeepDays 天的记录归档。MaxRecords 为 0 表示不自动归档。
type Policy struct {
	MaxRecords int `json:"maxRecords"`
	KeepDays   int `json:"keepDays"`
}

// 可选的值。
var (
	MaxRecordChoices = []int{0, 10000, 50000, 100000, 500000, 1000000}
	KeepDayChoices   = []int{30, 90, 180, 365, 730}
)

// DefaultPolicy 是没有设置过时的策略：不自动归档。
var DefaultPolicy = Policy{MaxRecords: 0, KeepDays: 365}

// Valid 报告策略是不是可选的值。
func (p Policy) Valid() bool {
	in := func(v int, list []int) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	return in(p.MaxRecords, MaxRecordChoices) && in(p.KeepDays, KeepDayChoices)
}

const policyKey = "audit.archive"

// LoadPolicy 读出归档策略；没有或不合法时用默认值。
func LoadPolicy(ctx context.Context, db *sql.DB) Policy {
	p := DefaultPolicy
	var v string
	if db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", policyKey).Scan(&v) == nil {
		var x Policy
		if json.Unmarshal([]byte(v), &x) == nil && x.Valid() {
			p = x
		}
	}
	return p
}

// SavePolicy 保存归档策略。调用方要先写审计。
func SavePolicy(ctx context.Context, db *sql.DB, p Policy) error {
	if !p.Valid() {
		return errors.New("归档条件不是可选的值")
	}
	v, _ := json.Marshal(p)
	_, err := db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, policyKey, string(v))
	return err
}

// Checkpoints 按时间先后返回全部检查点。
func Checkpoints(ctx context.Context, db *sql.DB) ([]Checkpoint, error) {
	rows, err := db.QueryContext(ctx, "SELECT upto, hash, first, file, sha256, records, created_at FROM audit_checkpoints ORDER BY upto")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Checkpoint{}
	for rows.Next() {
		var c Checkpoint
		if err := rows.Scan(&c.Upto, &c.Hash, &c.First, &c.File, &c.SHA256, &c.Records, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// lastCheckpoint 返回最新的检查点；没有时 ok 为 false。
func lastCheckpoint(ctx context.Context, db *sql.DB) (c Checkpoint, ok bool, err error) {
	err = db.QueryRowContext(ctx, "SELECT upto, hash, first, file, sha256, records, created_at FROM audit_checkpoints ORDER BY upto DESC LIMIT 1").
		Scan(&c.Upto, &c.Hash, &c.First, &c.File, &c.SHA256, &c.Records, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, false, nil
	}
	return c, err == nil, err
}

// Due 按策略算出这一次要归档到哪一条；0 表示不用归档。
func (l *Log) Due(ctx context.Context, p Policy, now time.Time) (int64, error) {
	if p.MaxRecords <= 0 {
		return 0, nil
	}
	var n int64
	if err := l.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit").Scan(&n); err != nil {
		return 0, err
	}
	if n <= int64(p.MaxRecords) {
		return 0, nil
	}
	cutoff := now.UTC().Add(-time.Duration(p.KeepDays) * 24 * time.Hour).Format(time.RFC3339Nano)
	var upto sql.NullInt64
	if err := l.db.QueryRowContext(ctx, "SELECT MAX(seq) FROM audit WHERE ts < ?", cutoff).Scan(&upto); err != nil {
		return 0, err
	}
	return upto.Int64, nil
}

// Maintain 读出策略，到了就归档一次。返回这次的检查点（没有归档时为 nil）。
func (l *Log) Maintain(ctx context.Context, dir string) (*Checkpoint, error) {
	upto, err := l.Due(ctx, LoadPolicy(ctx, l.db), time.Now())
	if err != nil || upto == 0 {
		return nil, err
	}
	return l.Archive(ctx, dir, upto)
}

// Archive 把 upto 及之前、还在数据库里的记录归档，然后从数据库里删掉。步骤见文件开头。
func (l *Log) Archive(ctx context.Context, dir string, upto int64) (*Checkpoint, error) {
	l.archiveMu.Lock()
	defer l.archiveMu.Unlock()

	prev, first := Genesis, int64(1)
	if cp, ok, err := lastCheckpoint(ctx, l.db); err != nil {
		return nil, err
	} else if ok {
		prev, first = cp.Hash, cp.Upto+1
	}
	if upto < first {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("建不了归档目录：%w", err)
	}
	name := fmt.Sprintf("audit-%010d-%010d.jsonl.gz", first, upto)
	tmp := filepath.Join(dir, name+".tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("写不了归档文件：%w", err)
	}
	defer os.Remove(tmp) // 成功时已经改名，这里删不到
	sum := sha256.New()
	zw := gzip.NewWriter(io.MultiWriter(f, sum))
	bw := bufio.NewWriter(zw)

	rows, err := l.db.QueryContext(ctx, "SELECT seq, ts, version, prev_hash, hash, body FROM audit WHERE seq BETWEEN ? AND ? ORDER BY seq", first, upto)
	if err != nil {
		f.Close()
		return nil, err
	}
	want, n, last := first, int64(0), ""
	for rows.Next() {
		var r Record
		var body string
		if err := rows.Scan(&r.Seq, &r.TS, &r.Version, &r.PrevHash, &r.Hash, &body); err != nil {
			rows.Close()
			f.Close()
			return nil, err
		}
		// 归档前再校验一遍：链断了的不归档，留在数据库里等人查。
		if r.Seq != want || r.PrevHash != prev || chainHash(prev, r.Seq, r.TS, r.Version, []byte(body)) != r.Hash {
			rows.Close()
			f.Close()
			return nil, fmt.Errorf("第 %d 条记录的哈希链对不上，不归档", r.Seq)
		}
		r.Body = json.RawMessage(body)
		line, _ := json.Marshal(r)
		bw.Write(append(line, '\n'))
		prev, last, want, n = r.Hash, r.Hash, want+1, n+1
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		f.Close()
		return nil, err
	}
	rows.Close()
	if n == 0 || want != upto+1 {
		f.Close()
		return nil, nil
	}
	if err := bw.Flush(); err == nil {
		err = zw.Close()
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, fmt.Errorf("写不了归档文件：%w", err)
	}
	final := filepath.Join(dir, name)
	if err := os.Rename(tmp, final); err != nil {
		return nil, fmt.Errorf("写不了归档文件：%w", err)
	}
	cp := &Checkpoint{Upto: upto, Hash: last, First: first, File: name, SHA256: hex.EncodeToString(sum.Sum(nil)), Records: n,
		CreatedAt: time.Now().UTC().Format(time.RFC3339)}

	// 先在链里留下这次归档：写不进去就不删。
	if _, err := l.Append(ctx, Entry{Type: "audit.archived", Entry: "system",
		Message: fmt.Sprintf("归档审计记录 %d～%d（%d 条）", first, upto, n),
		Detail:  map[string]any{"first": first, "upto": upto, "hash": last, "file": name, "sha256": cp.SHA256, "records": n}}); err != nil {
		return nil, fmt.Errorf("写审计失败，没有删除：%w", err)
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO audit_checkpoints (upto, hash, first, file, sha256, records, created_at) VALUES (?,?,?,?,?,?,?)",
		cp.Upto, cp.Hash, cp.First, cp.File, cp.SHA256, cp.Records, cp.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec("DELETE FROM audit WHERE seq <= ?", upto); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return cp, nil
}

var archiveNameRe = regexp.MustCompile(`^audit-[0-9]{10}-[0-9]{10}\.jsonl\.gz$`)

// ArchiveResult 是归档文件的校验结果。
type ArchiveResult struct {
	OK       bool   `json:"ok"`
	Archives int    `json:"archives"`
	Records  int64  `json:"records"`
	Message  string `json:"message,omitempty"`
}

// VerifyArchives 逐个校验归档文件：文件摘要和检查点一致、里面的链完整、首尾和前后的归档以及检查点接得上。
func VerifyArchives(ctx context.Context, db *sql.DB, dir string) (ArchiveResult, error) {
	cps, err := Checkpoints(ctx, db)
	if err != nil {
		return ArchiveResult{}, err
	}
	res := ArchiveResult{OK: true}
	prev, want := Genesis, int64(1)
	for _, cp := range cps {
		fail := func(msg string) (ArchiveResult, error) {
			res.OK, res.Message = false, cp.File+"："+msg
			return res, nil
		}
		if !archiveNameRe.MatchString(cp.File) {
			return fail("文件名不对")
		}
		if cp.First != want {
			return fail(fmt.Sprintf("应该从第 %d 条开始，实际从第 %d 条", want, cp.First))
		}
		raw, err := os.ReadFile(filepath.Join(dir, cp.File))
		if err != nil {
			return fail("读不到归档文件")
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != cp.SHA256 {
			return fail("文件摘要和检查点不一致（文件被改动或损坏）")
		}
		zr, err := gzip.NewReader(bytesReader(raw))
		if err != nil {
			return fail("不是有效的 gzip 文件")
		}
		sc := bufio.NewScanner(zr)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var r Record
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				return fail("有一行不是有效的记录")
			}
			if r.Seq != want || r.PrevHash != prev || chainHash(prev, r.Seq, r.TS, r.Version, r.Body) != r.Hash {
				return fail(fmt.Sprintf("第 %d 条记录的哈希链对不上", r.Seq))
			}
			prev, want = r.Hash, want+1
			res.Records++
		}
		if err := sc.Err(); err != nil {
			return fail("读文件出错")
		}
		if want != cp.Upto+1 || prev != cp.Hash {
			return fail("最后一条和检查点对不上")
		}
		res.Archives++
	}
	return res, nil
}
