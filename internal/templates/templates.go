package templates

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"syntroform/backend/internal/httpx"
)

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Routes(r chi.Router) {
	r.Get("/templates", s.list)
	r.Get("/templates/{id}", s.get)
	r.Post("/templates/{id}/use", s.use)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("category")
	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var q string
	var args []any
	if cat != "" {
		q = `SELECT id::text,name,description,category,est_minutes,snapshot FROM templates WHERE category=$1 ORDER BY name`
		args = append(args, cat)
	} else {
		q = `SELECT id::text,name,description,category,est_minutes,snapshot FROM templates ORDER BY category,name`
	}
	pgRows, _ := s.pool.Query(r.Context(), q, args...)
	defer pgRows.Close()
	var out []map[string]any
	for pgRows.Next() {
		var id, name, desc, category string
		var est int
		var snap []byte
		_ = pgRows.Scan(&id, &name, &desc, &category, &est, &snap)
		out = append(out, map[string]any{"id": id, "name": name, "description": desc, "category": category, "est_minutes": est, "snapshot": string(snap)})
	}
	// seed defaults if empty
	if len(out) == 0 {
		out = []map[string]any{
			{"id": "tmpl_1", "name": "Customer Satisfaction (CSAT)", "category": "Feedback", "est_minutes": 2},
			{"id": "tmpl_2", "name": "Job Application", "category": "Hiring", "est_minutes": 8},
			{"id": "tmpl_3", "name": "Event Registration", "category": "Events", "est_minutes": 3},
		}
	}
	_ = rows
	httpx.JSON(w, 200, out)
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var name, desc, cat string
	var est int
	var snap []byte
	err := s.pool.QueryRow(r.Context(), `SELECT name,description,category,est_minutes,snapshot FROM templates WHERE id=$1`, id).Scan(&name, &desc, &cat, &est, &snap)
	if err != nil {
		httpx.Error(w, 404, "NOT_FOUND", "template not found")
		return
	}
	httpx.JSON(w, 200, map[string]any{"id": id, "name": name, "description": desc, "category": cat, "est_minutes": est, "snapshot": string(snap)})
}

func (s *Service) use(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, map[string]any{"ok": true})
}
