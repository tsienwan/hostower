package gateway

import (
	"net/http"

	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/metrics"
)

// 指标历史的保留时长。看任何登录的人都可以；改只有所有者能做，并且先写审计、写不进去就不改。

func (s *Server) retention(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		if !ownerOnly(w, r) {
			return
		}
		var in metrics.Retention
		if !readJSON(w, r, &in) {
			return
		}
		if !in.Valid() {
			writeErr(w, 400, "保留时长不是可选的值")
			return
		}
		old := s.Sampler.Retention()
		if old != in {
			sess := sessionOf(r)
			if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "system.retention", Actor: sess.User.Name, Entry: "web",
				Message: "修改指标历史的保留时长",
				Detail:  map[string]any{"ip": s.clientIP(r), "before": old, "after": in}}); err != nil {
				writeErr(w, 500, "写审计失败，没有修改")
				return
			}
			if err := s.Sampler.SetRetention(in); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
	}
	n, _ := s.Sampler.Stored(r.Context())
	writeJSON(w, 200, map[string]any{"retention": s.Sampler.Retention(), "fine": metrics.FineChoices, "coarse": metrics.CoarseChoices, "stored": n})
}
