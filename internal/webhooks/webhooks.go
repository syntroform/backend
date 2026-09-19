package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"syntroform/backend/internal/auth"
	"syntroform/backend/internal/httpx"
)

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Routes(r chi.Router) {
	r.Get("/workspaces/{id}/webhooks", s.list)
	r.Post("/workspaces/{id}/webhooks", s.create)
	r.Delete("/webhooks/{id}", s.delete)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	ws := chi.URLParam(r, "id")
	rows, _ := s.pool.Query(r.Context(), `SELECT id::text,url,events,active,created_at FROM webhooks WHERE workspace_id=$1`, ws)
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, url string
		var events []string
		var active bool
		var created time.Time
		_ = rows.Scan(&id, &url, &events, &active, &created)
		out = append(out, map[string]any{"id": id, "url": url, "events": events, "active": active, "created_at": created})
	}
	if out == nil {
		out = []map[string]any{}
	}
	httpx.JSON(w, 200, out)
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	ws := chi.URLParam(r, "id")
	uid, _ := auth.UserID(r)
	_ = uid
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
		FormID *string  `json:"form_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.URL == "" {
		httpx.Error(w, 400, "VALIDATION", "url required")
		return
	}
	secret := "whsec_" + random(24)
	var id string
	err := s.pool.QueryRow(r.Context(), `INSERT INTO webhooks(workspace_id,url,secret,events) VALUES($1,$2,$3,$4) RETURNING id::text`, ws, req.URL, secret, req.Events).Scan(&id)
	if err != nil {
		httpx.Error(w, 500, "INTERNAL", err.Error())
		return
	}
	httpx.JSON(w, 201, map[string]any{"id": id, "secret": secret})
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, _ = s.pool.Exec(r.Context(), `DELETE FROM webhooks WHERE id=$1`, id)
	httpx.JSON(w, 200, map[string]any{"ok": true})
}

// Delivery with HMAC
func Deliver(pool *pgxpool.Pool) {
	// background worker loop: poll pending deliveries
	go func() {
		for {
			time.Sleep(5 * time.Second)
			rows, err := pool.Query(contextBackground(), `SELECT wd.id::text, w.url, w.secret, wd.payload, wd.attempts FROM webhook_deliveries wd JOIN webhooks w ON w.id=wd.webhook_id WHERE wd.status='pending' AND wd.attempts < 5 LIMIT 10`)
			if err != nil {
				continue
			}
			for rows.Next() {
				var id, url, secret string
				var payload []byte
				var attempts int
				_ = rows.Scan(&id, &url, &secret, &payload, &attempts)
				sig := sign(payload, secret)
				req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Syntroform-Signature", sig)
				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Do(req)
				if err != nil || resp.StatusCode >= 300 {
					_, _ = pool.Exec(contextBackground(), `UPDATE webhook_deliveries SET attempts=attempts+1, last_error=$1, status=CASE WHEN attempts+1>=5 THEN 'failed' ELSE 'pending' END WHERE id=$2`, errStr(err, resp), id)
					if resp != nil {
						resp.Body.Close()
					}
				} else {
					_, _ = pool.Exec(contextBackground(), `UPDATE webhook_deliveries SET status='delivered', attempts=attempts+1 WHERE id=$1`, id)
					resp.Body.Close()
				}
			}
			rows.Close()
		}
	}()
}

func sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func random(n int) string {
	b := make([]byte, n)
	_, _ = bytes.NewReader(b).Read(b)
	return hex.EncodeToString(b)[:n]
}

func errStr(err error, resp *http.Response) string {
	if err != nil {
		return err.Error()
	}
	if resp != nil {
		return "status " + resp.Status
	}
	return "unknown"
}

func contextBackground() context.Context { return context.TODO() }
