package audit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/store"
)

func TestArchive(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	log := audit.Open(db, nil)
	for i := 0; i < 30; i++ {
		if _, err := log.Append(ctx, audit.Entry{Type: "test", Message: "第几条"}); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()

	// 策略没打开：不归档；打开了、条数也超了，但都不够旧：也不归档。
	if upto, _ := log.Due(ctx, audit.DefaultPolicy, time.Now()); upto != 0 {
		t.Fatal("默认不应该归档")
	}
	p := audit.Policy{MaxRecords: 10000, KeepDays: 30}
	if upto, _ := log.Due(ctx, p, time.Now().Add(365*24*time.Hour)); upto != 0 {
		t.Fatal("条数没超过上限时不应该归档")
	}

	// 数据库里的记录不能随便删。
	if _, err := db.Exec("DELETE FROM audit WHERE seq = 1"); err == nil {
		t.Fatal("没归档的记录不应该能删")
	}

	cp, err := log.Archive(ctx, dir, 10)
	if err != nil || cp == nil || cp.First != 1 || cp.Upto != 10 || cp.Records != 10 {
		t.Fatalf("归档不对：%+v %v", cp, err)
	}
	res, err := audit.Verify(ctx, db)
	if err != nil || !res.OK || res.Archived != 10 || res.Count != 21 { // 20 条剩下的 + 1 条归档记录
		t.Fatalf("归档后数据库里的链应该从检查点接着校验通过：%+v %v", res, err)
	}
	if _, err := db.Exec("DELETE FROM audit WHERE seq = 11"); err == nil {
		t.Fatal("检查点之后的记录仍然不能删")
	}
	if _, err := db.Exec("DELETE FROM audit_checkpoints"); err == nil {
		t.Fatal("检查点不能删")
	}

	// 再归档一段：和上一段首尾相接。
	if _, err := log.Archive(ctx, dir, 20); err != nil {
		t.Fatal(err)
	}
	ar, err := audit.VerifyArchives(ctx, db, dir)
	if err != nil || !ar.OK || ar.Archives != 2 || ar.Records != 20 {
		t.Fatalf("归档文件应该都校验通过：%+v %v", ar, err)
	}
	if res, _ := audit.Verify(ctx, db); !res.OK || res.Archived != 20 {
		t.Fatalf("第二次归档后数据库里的链也应该完整：%+v", res)
	}
	// 新的记录照常接在后面。
	if seq, err := log.Append(ctx, audit.Entry{Type: "test"}); err != nil || seq != 33 {
		t.Fatalf("归档后追加的序号不对：%d %v", seq, err)
	}

	// 改动归档文件：校验能发现。
	cps, _ := audit.Checkpoints(ctx, db)
	path := filepath.Join(dir, cps[0].File)
	raw, _ := os.ReadFile(path)
	raw[len(raw)/2] ^= 0xff
	os.WriteFile(path, raw, 0o600)
	if ar, _ := audit.VerifyArchives(ctx, db, dir); ar.OK {
		t.Fatal("改动过的归档文件应该校验失败")
	}

	// 按策略：很久以后、条数也超了，就归档到最新的那些旧记录。
	if upto, _ := log.Due(ctx, audit.Policy{MaxRecords: 10000, KeepDays: 30}, time.Now()); upto != 0 {
		t.Fatal("条数没到上限不应该归档")
	}
	// 条数超过上限（这里用一个很小的上限）：一年以后看，现在的记录都够旧了，归档到最后一条。
	if upto, _ := log.Due(ctx, audit.Policy{MaxRecords: 5, KeepDays: 30}, time.Now().Add(365*24*time.Hour)); upto != 33 {
		t.Fatalf("应该归档到第 33 条，实际 %d", upto)
	}
	// 记录还不够旧：不归档。
	if upto, _ := log.Due(ctx, audit.Policy{MaxRecords: 5, KeepDays: 30}, time.Now()); upto != 0 {
		t.Fatalf("不够旧的记录不应该归档，实际 %d", upto)
	}
}
