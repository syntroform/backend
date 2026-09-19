package analytics

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"syntroform/backend/internal/auth"
	"syntroform/backend/internal/httpx"
)

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Routes(r chi.Router) {
	r.Get("/forms/{id}/analytics", s.overview)
	r.Get("/forms/{id}/analytics/questions", s.questions)
	r.Get("/forms/{id}/analytics/funnel", s.funnel)
}

func (s *Service) overview(w http.ResponseWriter, r *http.Request) {
	formID := chi.URLParam(r, "id")
	uid, _ := auth.UserID(r)
	var ws string
	_ = s.pool.QueryRow(r.Context(), `SELECT workspace_id::text FROM forms WHERE id=$1`, formID).Scan(&ws)
	var ok bool
	_ = s.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=$1 AND user_id=$2)`, ws, uid).Scan(&ok)
	if !ok {
		httpx.Error(w, 403, "FORBIDDEN", "no access")
		return
	}
	// counts
	var views, starts, completions, respCount int
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM analytics_events WHERE form_id=$1 AND event_type='form_view'`, formID).Scan(&views)
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM analytics_events WHERE form_id=$1 AND event_type='form_start'`, formID).Scan(&starts)
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM analytics_events WHERE form_id=$1 AND event_type='form_complete'`, formID).Scan(&completions)
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM responses WHERE form_id=$1`, formID).Scan(&respCount)

	if completions < respCount {
		completions = respCount
	}
	if views < completions {
		views = completions
	}
	if starts < completions {
		starts = completions
	}

	var avgMs, medianMs int
	_ = s.pool.QueryRow(r.Context(), `SELECT COALESCE(AVG(completion_sec),0)::int, COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY completion_sec),0)::int FROM responses WHERE form_id=$1`, formID).Scan(&avgMs, &medianMs)

	// daily series
	rows, _ := s.pool.Query(r.Context(), `SELECT day, views, starts, completions FROM form_daily_stats WHERE form_id=$1 ORDER BY day`, formID)
	var series []map[string]any
	for rows.Next() {
		var day string
		var v, st, c int
		_ = rows.Scan(&day, &v, &st, &c)
		series = append(series, map[string]any{"day": day, "views": v, "starts": st, "completions": c})
	}
	rows.Close()
	// device & source
	drows, _ := s.pool.Query(r.Context(), `SELECT device, COUNT(*) FROM analytics_events WHERE form_id=$1 GROUP BY device`, formID)
	var devices []map[string]any
	for drows.Next() {
		var dev string
		var cnt int
		_ = drows.Scan(&dev, &cnt)
		devices = append(devices, map[string]any{"device": dev, "count": cnt})
	}
	drows.Close()

	httpx.JSON(w, 200, map[string]any{
		"views": views, "starts": starts, "completions": completions,
		"completion_rate": func() float64 {
			if views == 0 {
				return 0
			}
			return float64(completions) / float64(views) * 100
		}(),
		"avg_sec": avgMs, "median_sec": medianMs,
		"series": series, "devices": devices,
	})
}

func (s *Service) questions(w http.ResponseWriter, r *http.Request) {
	formID := chi.URLParam(r, "id")
	rows, _ := s.pool.Query(r.Context(), `
		SELECT q.id::text, q.title,
		       (SELECT COUNT(*) FROM analytics_events WHERE question_id=q.id AND event_type='question_view'),
		       (SELECT COUNT(*) FROM analytics_events WHERE question_id=q.id AND event_type='question_answer'),
		       (SELECT COUNT(*) FROM analytics_events WHERE question_id=q.id AND event_type='question_skip')
		FROM form_questions q WHERE q.form_id=$1 ORDER BY q.position`, formID)
	var out []map[string]any
	for rows.Next() {
		var qid, title string
		var views, answers, skips int
		_ = rows.Scan(&qid, &title, &views, &answers, &skips)
		drop := views - answers - skips
		if drop < 0 {
			drop = 0
		}
		out = append(out, map[string]any{"question_id": qid, "title": title, "views": views, "answers": answers, "skips": skips, "dropoffs": drop})
	}
	rows.Close()
	if out == nil {
		out = []map[string]any{}
	}
	// distribution for choice questions
	httpx.JSON(w, 200, out)
}

func (s *Service) funnel(w http.ResponseWriter, r *http.Request) {
	formID := chi.URLParam(r, "id")
	var views, starts, completes int
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM analytics_events WHERE form_id=$1 AND event_type='form_view'`, formID).Scan(&views)
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM analytics_events WHERE form_id=$1 AND event_type='form_start'`, formID).Scan(&starts)
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM analytics_events WHERE form_id=$1 AND event_type='form_complete'`, formID).Scan(&completes)
	// reached section/question counts
	var reachedQ int
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(DISTINCT session_id) FROM analytics_events WHERE form_id=$1 AND event_type='question_view'`, formID).Scan(&reachedQ)
	httpx.JSON(w, 200, map[string]any{
		"funnel": []map[string]any{
			{"stage": "Visited", "count": views},
			{"stage": "Started", "count": starts},
			{"stage": "Reached question", "count": reachedQ},
			{"stage": "Completed", "count": completes},
		},
	})
}

var _ = json.RawMessage{}
