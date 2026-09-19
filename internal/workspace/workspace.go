package workspace

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
	r.Get("/workspaces", s.list)
	r.Post("/workspaces", s.create)
	r.Get("/workspaces/{id}/members", s.members)
	r.Post("/workspaces/{id}/members", s.invite)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	rows, err := s.pool.Query(r.Context(), `SELECT w.id::text,w.name,w.slug,wm.role FROM workspaces w JOIN workspace_members wm ON wm.workspace_id=w.id WHERE wm.user_id=$1`, uid)
	if err != nil {
		httpx.Error(w, 500, "INTERNAL", "db error")
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, slug, role string
		_ = rows.Scan(&id, &name, &slug, &role)
		out = append(out, map[string]any{"id": id, "name": name, "slug": slug, "role": role})
	}
	httpx.JSON(w, 200, out)
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		httpx.Error(w, 400, "VALIDATION", "name required")
		return
	}
	var id, slug string
	slug = req.Name
	// naive slug
	for i := range slug {
		if slug[i] == ' ' {
			slug = slug[:i] + "-" + slug[i+1:]
		}
	}
	err := s.pool.QueryRow(r.Context(), `INSERT INTO workspaces(name,slug,owner_id) VALUES($1,$2,$3) RETURNING id::text, slug`, req.Name, slug, uid).Scan(&id, &slug)
	if err != nil {
		httpx.Error(w, 500, "INTERNAL", err.Error())
		return
	}
	_, _ = s.pool.Exec(r.Context(), `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'Owner')`, id, uid)
	httpx.JSON(w, 201, map[string]any{"id": id, "name": req.Name, "slug": slug})
}

func (s *Service) members(w http.ResponseWriter, r *http.Request) {
	wsID := chi.URLParam(r, "id")
	rows, _ := s.pool.Query(r.Context(), `SELECT u.id::text,u.name,u.email,wm.role FROM workspace_members wm JOIN users u ON u.id=wm.user_id WHERE wm.workspace_id=$1`, wsID)
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, email, role string
		_ = rows.Scan(&id, &name, &email, &role)
		out = append(out, map[string]any{"id": id, "name": name, "email": email, "role": role})
	}
	httpx.JSON(w, 200, out)
}

func (s *Service) invite(w http.ResponseWriter, r *http.Request) {
	// placeholder: create pending
	httpx.JSON(w, 200, map[string]any{"ok": true})
}
