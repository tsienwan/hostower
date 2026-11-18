package gateway

import (
	"context"
	"net/http"
	"time"

	"github.com/tsienwan/hostower/internal/kernel/audit"
)

// 审计归档：看策略和已有的归档谁都可以；改策略、立即执行只有所有者能做。
// 改策略先写审计，写不进去就不改；每次归档本身也会在链里留下一条 audit.archived。

func (s *Server) auditArchiveView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cps, err := audit.Checkpoints(ctx, s.DB)
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	var archived int64
	for _, c := range cps {
		archived += c.Records
	}
	var inDB int64
	s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit").Scan(&inDB)
	// 新的在前，最多列 50 个。
	recent := []audit.Checkpoint{}
	for i := len(cps) - 1; i >= 0 && len(recent) < 50; i-- {
		recent = append(recent, cps[i])
	}
	writeJSON(w, 200, map[string]any{
		"policy": audit.LoadPolicy(ctx, s.DB), "maxRecordChoices": audit.MaxRecordChoices, "keepDayChoices": audit.KeepDayChoices,
		"checkpoints": recent, "archives": len(cps), "archivedRecords": archived, "records": inDB,
	})
}

func (s *Server) auditArchiveSet(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in audit.Policy
	if !readJSON(w, r, &in) {
		return
	}
	if !in.Valid() {
		writeErr(w, 400, "归档条件不是可选的值")
		return
	}
	before := audit.LoadPolicy(r.Context(), s.DB)
	if before != in {
		sess := sessionOf(r)
		if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "audit.retention", Actor: sess.User.Name, Entry: "web",
			Message: "修改审计归档条件", Detail: map[string]any{"ip": s.clientIP(r), "before": before, "after": in}}); err != nil {
			writeErr(w, 500, "写审计失败，没有修改")
			return
		}
		if err := audit.SavePolicy(r.Context(), s.DB, in); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		// 新条件可能马上就满足：在后台跑一次。
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if _, err := s.Kernel.Audit().Maintain(ctx, s.AuditArchiveDir); err != nil {
				s.Log.Error("审计归档失败", "err", err)
			}
		}()
	}
	s.auditArchiveView(w, r)
}

func (s *Server) auditArchiveRun(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	cp, err := s.Kernel.Audit().Maintain(ctx, s.AuditArchiveDir)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"checkpoint": cp})
}
