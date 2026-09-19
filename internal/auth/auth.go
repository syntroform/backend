package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"syntroform/backend/internal/httpx"
	mw "syntroform/backend/internal/middleware"
)

type Service struct {
	pool      *pgxpool.Pool
	jwtSecret []byte
}

func New(pool *pgxpool.Pool, secret string) *Service {
	return &Service{pool: pool, jwtSecret: []byte(secret)}
}

func (s *Service) Routes(r chi.Router) {
	r.Post("/auth/register", s.register)
	r.Post("/auth/login", s.login)
	r.Post("/auth/logout", s.logout)
	r.Get("/auth/me", s.withAuth(s.me))
}

func (s *Service) withAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := tokenFromRequest(r)
		if token == "" {
			httpx.Error(w, 401, "UNAUTHORIZED", "missing token")
			return
		}
		claims := &Claims{}
		_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) { return s.jwtSecret, nil })
		if err != nil {
			httpx.Error(w, 401, "UNAUTHORIZED", "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), mw.UserIDKey, claims.UserID)
		h(w, r.WithContext(ctx))
	}
}

func tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie("fc_token"); err == nil && c.Value != "" {
		return c.Value
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

type Claims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

func (s *Service) sign(userID string) (string, error) {
	claims := Claims{UserID: userID, RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(s.jwtSecret)
}

func setCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "fc_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
}

// Handlers

type registerReq struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, 400, "BAD_REQUEST", "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || len(req.Password) < 8 || req.Name == "" {
		httpx.Error(w, 400, "VALIDATION_ERROR", "name, valid email and 8+ char password required")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	var id string
	err := s.pool.QueryRow(r.Context(), `INSERT INTO users(email,password_hash,name) VALUES($1,$2,$3) RETURNING id::text`, req.Email, string(hash), req.Name).Scan(&id)
	if err != nil {
		httpx.Error(w, 409, "EMAIL_TAKEN", "email already registered")
		return
	}
	// create workspace for user
	wsID := uuid.NewString()
	slug := strings.ReplaceAll(strings.ToLower(req.Name), " ", "-") + "-" + id[:8]
	_, err = s.pool.Exec(r.Context(), `INSERT INTO workspaces(id,name,slug,owner_id) VALUES($1,$2,$3,$4)`, wsID, req.Name+"'s Workspace", slug, id)
	if err != nil {
		httpx.Error(w, 500, "INTERNAL", "workspace create failed")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'Owner')`, wsID, id)
	token, _ := s.sign(id)
	setCookie(w, token, false)
	httpx.JSON(w, 201, map[string]any{"id": id, "email": req.Email, "name": req.Name, "token": token})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, 400, "BAD_REQUEST", "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	var id, hash, name string
	err := s.pool.QueryRow(r.Context(), `SELECT id::text,password_hash,name FROM users WHERE email=$1`, req.Email).Scan(&id, &hash, &name)
	if err == sql.ErrNoRows || err != nil {
		httpx.Error(w, 401, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		httpx.Error(w, 401, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}
	token, _ := s.sign(id)
	setCookie(w, token, false)
	httpx.JSON(w, 200, map[string]any{"id": id, "email": req.Email, "name": name, "token": token})
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "fc_token", Value: "", Path: "/", MaxAge: -1})
	httpx.JSON(w, 200, map[string]any{"ok": true})
}

func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(mw.UserIDKey).(string)
	var email, name string
	err := s.pool.QueryRow(r.Context(), `SELECT email,name FROM users WHERE id=$1`, uid).Scan(&email, &name)
	if err != nil {
		httpx.Error(w, 404, "NOT_FOUND", "user not found")
		return
	}
	rows, _ := s.pool.Query(r.Context(), `SELECT w.id::text,w.name,w.slug,wm.role FROM workspaces w JOIN workspace_members wm ON wm.workspace_id=w.id WHERE wm.user_id=$1`, uid)
	defer rows.Close()
	var workspaces []map[string]any
	for rows.Next() {
		var id, n, slug, role string
		_ = rows.Scan(&id, &n, &slug, &role)
		workspaces = append(workspaces, map[string]any{"id": id, "name": n, "slug": slug, "role": role})
	}
	httpx.JSON(w, 200, map[string]any{"id": uid, "email": email, "name": name, "workspaces": workspaces})
}

// Helper to get user id from context
func UserID(r *http.Request) (string, bool) {
	v := r.Context().Value(mw.UserIDKey)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func RequireAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := tokenFromRequest(r)
			if tok == "" {
				httpx.Error(w, 401, "UNAUTHORIZED", "auth required")
				return
			}
			claims := &Claims{}
			_, err := jwt.ParseWithClaims(tok, claims, func(t *jwt.Token) (any, error) { return []byte(secret), nil })
			if err != nil {
				httpx.Error(w, 401, "UNAUTHORIZED", "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), mw.UserIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
