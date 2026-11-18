// Package audit 是只追加的审计日志。每条记录包含上一条的哈希，
// 篡改或删除任何一条都会让后面的链断开。这里不提供修改接口；
// 唯一的删除路径是归档（archive.go）：先写带摘要的归档文件和检查点，再删检查点之前的记录。
package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FormatVersion 是审计记录的格式版本。历史记录不改写，只在读取时转换。
const FormatVersion = 1

// Genesis 是第一条记录的 prev_hash。
const Genesis = "0000000000000000000000000000000000000000000000000000000000000000"

// Entry 是一条审计记录的内容。
type Entry struct {
	Type      string         `json:"type"`
	Actor     string         `json:"actor,omitempty"`
	ActorKind string         `json:"actorKind,omitempty"`
	Entry     string         `json:"entry,omitempty"`
	RequestID string         `json:"requestId,omitempty"`
	Parent    int64          `json:"parent,omitempty"`
	Host      string         `json:"host,omitempty"`
	Intent    string         `json:"intent,omitempty"`
	Command   string         `json:"command,omitempty"`
	Level     string         `json:"level,omitempty"`
	Reasons   []string       `json:"reasons,omitempty"`
	Approver  string         `json:"approver,omitempty"`
	ExitCode  *int           `json:"exitCode,omitempty"`
	Output    string         `json:"output,omitempty"`
	Backup    string         `json:"backup,omitempty"`
	Snippet   string         `json:"snippet,omitempty"`
	Message   string         `json:"message,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// Record 是落盘后的一条记录。
type Record struct {
	Seq      int64           `json:"seq"`
	TS       string          `json:"ts"`
	Version  int             `json:"version"`
	PrevHash string          `json:"prevHash"`
	Hash     string          `json:"hash"`
	Body     json.RawMessage `json:"body"`
}

// Sink 是审计的外部副本接收端。
type Sink interface {
	Write(Record) error
}

// Log 是审计日志。
type Log struct {
	db        *sql.DB
	mu        sync.Mutex
	archiveMu sync.Mutex // 同一时间只做一次归档
	sinks     []Sink
	onErr     func(error)
}

// Open 打开审计日志。
func Open(db *sql.DB, onSinkError func(error), sinks ...Sink) *Log {
	if onSinkError == nil {
		onSinkError = func(error) {}
	}
	return &Log{db: db, sinks: sinks, onErr: onSinkError}
}

func chainHash(prev string, seq int64, ts string, version int, body []byte) string {
	h := sha256.New()
	io.WriteString(h, prev)
	io.WriteString(h, "\n"+strconv.FormatInt(seq, 10)+"\n"+ts+"\n"+strconv.Itoa(version)+"\n")
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// Append 追加一条记录并返回它的序号。写审计失败时调用方必须中止操作。
func (l *Log) Append(ctx context.Context, e Entry) (int64, error) {
	body, err := json.Marshal(e)
	if err != nil {
		return 0, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var seq int64
	prev := Genesis
	err = tx.QueryRow("SELECT seq, hash FROM audit ORDER BY seq DESC LIMIT 1").Scan(&seq, &prev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	rec := Record{
		Seq:      seq + 1,
		TS:       time.Now().UTC().Format(time.RFC3339Nano),
		Version:  FormatVersion,
		PrevHash: prev,
		Body:     body,
	}
	rec.Hash = chainHash(prev, rec.Seq, rec.TS, rec.Version, body)
	if _, err := tx.Exec("INSERT INTO audit (seq, ts, version, prev_hash, hash, body) VALUES (?,?,?,?,?,?)",
		rec.Seq, rec.TS, rec.Version, rec.PrevHash, rec.Hash, string(body)); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	for _, s := range l.sinks {
		if err := s.Write(rec); err != nil {
			l.onErr(err)
		}
	}
	return rec.Seq, nil
}

// List 按序号倒序返回记录。before <= 0 表示从最新开始。
func (l *Log) List(ctx context.Context, before int64, limit int) ([]Record, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := "SELECT seq, ts, version, prev_hash, hash, body FROM audit"
	var args []any
	if before > 0 {
		q += " WHERE seq < ?"
		args = append(args, before)
	}
	q += " ORDER BY seq DESC LIMIT ?"
	args = append(args, limit)
	rows, err := l.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		var body string
		if err := rows.Scan(&r.Seq, &r.TS, &r.Version, &r.PrevHash, &r.Hash, &body); err != nil {
			return nil, err
		}
		r.Body = json.RawMessage(body)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Routine 是“日常查看”类的记录：翻目录、页面上的只读查询（列容器、看日志、看容器配置、搜文件）。
// 它们要不要写进审计由所有者的一个设置决定，默认不写；写了的话，页面也默认不显示，免得把关键操作淹没。
var Routine = []string{"file.list", "query"}

// Filter 是页面查询审计的条件。
type Filter struct {
	// Text 在记录内容里搜索（类型、人、主机、命令、说明都在里面）。
	Text string
	// Types 是和搜索词对得上的事件类型：记录的类型在里面也算匹配。只在 Text 不为空时有意义。
	Types []string
	// All 为 false 时不返回日常查看类的记录。
	All    bool
	Limit  int
	Offset int
}

// Page 按条件查询一页，最新的在前；同时返回符合条件的总数。
func (l *Log) Page(ctx context.Context, f Filter) ([]Record, int, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	where, args := "1=1", []any{}
	if !f.All {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(Routine)), ",")
		where += " AND json_extract(body, '$.type') NOT IN (" + marks + ")"
		for _, t := range Routine {
			args = append(args, t)
		}
	}
	if text := strings.TrimSpace(f.Text); text != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text)
		cond := "body LIKE ? ESCAPE '\\'"
		args = append(args, "%"+esc+"%")
		if len(f.Types) > 0 {
			cond += " OR json_extract(body, '$.type') IN (" + strings.TrimSuffix(strings.Repeat("?,", len(f.Types)), ",") + ")"
			for _, t := range f.Types {
				args = append(args, t)
			}
		}
		where += " AND (" + cond + ")"
	}
	var total int
	if err := l.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := l.db.QueryContext(ctx, "SELECT seq, ts, version, prev_hash, hash, body FROM audit WHERE "+where+" ORDER BY seq DESC LIMIT ? OFFSET ?",
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		var body string
		if err := rows.Scan(&r.Seq, &r.TS, &r.Version, &r.PrevHash, &r.Hash, &body); err != nil {
			return nil, 0, err
		}
		r.Body = json.RawMessage(body)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// VerifyResult 是哈希链校验结果。
type VerifyResult struct {
	OK    bool  `json:"ok"`
	Count int64 `json:"count"`
	// Archived 是已经归档、不在数据库里的记录到哪一条为止（0 表示没有归档过）。校验从它后面接着算。
	Archived int64  `json:"archived,omitempty"`
	Head     string `json:"head"`
	BrokenAt int64  `json:"brokenAt,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Verify 校验数据库里的哈希链：没有归档过就从第一条开始，归档过就从最新的检查点接着算。
// 可以离线对着数据库文件跑；归档文件用 VerifyArchives 校验。
func Verify(ctx context.Context, db *sql.DB) (VerifyResult, error) {
	res := VerifyResult{OK: true, Head: Genesis}
	prev := Genesis
	var want int64 = 1
	if cp, ok, err := lastCheckpoint(ctx, db); err != nil {
		return res, err
	} else if ok {
		prev, want, res.Archived, res.Head = cp.Hash, cp.Upto+1, cp.Upto, cp.Hash
	}
	rows, err := db.QueryContext(ctx, "SELECT seq, ts, version, prev_hash, hash, body FROM audit ORDER BY seq ASC")
	if err != nil {
		return VerifyResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var r Record
		var body string
		if err := rows.Scan(&r.Seq, &r.TS, &r.Version, &r.PrevHash, &r.Hash, &body); err != nil {
			return res, err
		}
		fail := func(msg string) (VerifyResult, error) {
			res.OK, res.BrokenAt, res.Message = false, r.Seq, msg
			return res, nil
		}
		if r.Seq != want {
			return fail(fmt.Sprintf("序号不连续：期望 %d，实际 %d（有记录被删除）", want, r.Seq))
		}
		if r.PrevHash != prev {
			return fail("prev_hash 与上一条记录的哈希不一致")
		}
		if chainHash(prev, r.Seq, r.TS, r.Version, []byte(body)) != r.Hash {
			return fail("记录内容与哈希不一致（内容被改动）")
		}
		prev = r.Hash
		want++
		res.Count++
	}
	res.Head = prev
	return res, rows.Err()
}

// bytesReader 让 gzip 从内存里的字节读。
func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
