package audit_test

import (
	"context"
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/store"
)

func TestChain(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log := audit.Open(db, nil)
	for i := 0; i < 5; i++ {
		if _, err := log.Append(ctx, audit.Entry{Type: "test", Command: "df -h"}); err != nil {
			t.Fatal(err)
		}
	}
	res, err := audit.Verify(ctx, db)
	if err != nil || !res.OK || res.Count != 5 {
		t.Fatalf("链应完整: %+v %v", res, err)
	}

	// 正常接口改不了也删不掉。
	if _, err := db.Exec("UPDATE audit SET body = '{}' WHERE seq = 3"); err == nil {
		t.Fatal("UPDATE 应被触发器拒绝")
	}
	if _, err := db.Exec("DELETE FROM audit WHERE seq = 3"); err == nil {
		t.Fatal("DELETE 应被触发器拒绝")
	}

	// 绕过触发器直接改库文件：校验必须发现。
	if _, err := db.Exec("DROP TRIGGER audit_no_update"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE audit SET body = '{"type":"forged"}' WHERE seq = 3`); err != nil {
		t.Fatal(err)
	}
	res, err = audit.Verify(ctx, db)
	if err != nil || res.OK || res.BrokenAt != 3 {
		t.Fatalf("应在第 3 条发现篡改: %+v %v", res, err)
	}
}

func TestDeleteDetected(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log := audit.Open(db, nil)
	for i := 0; i < 4; i++ {
		log.Append(ctx, audit.Entry{Type: "test"})
	}
	db.Exec("DROP TRIGGER audit_no_delete")
	db.Exec("DELETE FROM audit WHERE seq = 2")
	res, _ := audit.Verify(ctx, db)
	if res.OK || res.BrokenAt != 3 {
		t.Fatalf("应发现第 2 条被删: %+v", res)
	}
}

// 页面查询：默认不显示日常查看类的记录，可以搜索，可以分页；记录本身一条都不少。
func TestPage(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	log := audit.Open(db, nil)
	for i := 0; i < 5; i++ {
		log.Append(ctx, audit.Entry{Type: "file.list", Host: "web-1", Command: "列目录 /etc"})
		log.Append(ctx, audit.Entry{Type: "query", Host: "web-1", Command: "docker ps"})
		log.Append(ctx, audit.Entry{Type: "approval.granted", Host: "web-1", Command: "systemctl restart nginx"})
	}
	log.Append(ctx, audit.Entry{Type: "exec.host", Host: "db-1", Command: "echo 100%_done"})

	recs, total, err := log.Page(ctx, audit.Filter{Limit: 4})
	if err != nil || total != 6 || len(recs) != 4 {
		t.Fatalf("关键操作：total=%d len=%d err=%v", total, len(recs), err)
	}
	if recs[0].Seq < recs[1].Seq {
		t.Fatal("应该最新的在前")
	}
	if recs, total, _ = log.Page(ctx, audit.Filter{Limit: 4, Offset: 4}); total != 6 || len(recs) != 2 {
		t.Fatalf("第二页：total=%d len=%d", total, len(recs))
	}
	if _, total, _ = log.Page(ctx, audit.Filter{All: true}); total != 16 {
		t.Fatalf("全部：total=%d", total)
	}
	if _, total, _ = log.Page(ctx, audit.Filter{Text: "nginx"}); total != 5 {
		t.Fatalf("搜索 nginx：total=%d", total)
	}
	if _, total, _ = log.Page(ctx, audit.Filter{Text: "docker ps", All: true}); total != 5 {
		t.Fatalf("在全部里搜索：total=%d", total)
	}
	// 按页面上显示的事件名搜索：词本身不在记录里，但对应的事件类型在。
	if _, total, _ = log.Page(ctx, audit.Filter{Text: "批准", Types: []string{"approval.granted"}}); total != 5 {
		t.Fatalf("按事件名搜索：total=%d", total)
	}
	// % 和 _ 按字面搜索，不当通配符。
	if _, total, _ = log.Page(ctx, audit.Filter{Text: "100%_done"}); total != 1 {
		t.Fatalf("搜索带 %% 的词：total=%d", total)
	}
	if _, total, _ = log.Page(ctx, audit.Filter{Text: "1%e"}); total != 0 {
		t.Fatalf("%% 不应该当通配符：total=%d", total)
	}
	if res, err := audit.Verify(ctx, db); err != nil || !res.OK || res.Count != 16 {
		t.Fatalf("哈希链：%+v %v", res, err)
	}
}
