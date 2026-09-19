package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"

	"syntroform/backend/internal/analytics"
	"syntroform/backend/internal/auth"
	"syntroform/backend/internal/config"
	"syntroform/backend/internal/db"
	"syntroform/backend/internal/forms"
	"syntroform/backend/internal/jobs"
	"syntroform/backend/internal/middleware"
	"syntroform/backend/internal/responses"
	"syntroform/backend/internal/templates"
	"syntroform/backend/internal/webhooks"
	"syntroform/backend/internal/workspace"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database unavailable: %v\nStart PostgreSQL and retry (for example: brew services start postgresql@16), or set DATABASE_URL to a reachable PostgreSQL instance.", err)
	} else {
		if err := db.RunMigrations(ctx, pool, "migrations"); err != nil {
			log.Printf("migrations failed: %v", err)
		} else {
			log.Println("migrations ok")
		}
		jobs.Start(ctx, pool)
		webhooks.Deliver(pool)
	}
	devUserID := ""
	if cfg.Env == "development" {
		devUserID = ensureDevelopmentAccount(ctx, pool)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.RateLimit)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.FrontendURL, "http://localhost:3000", "http://localhost:3001", "http://127.0.0.1:3000", "http://127.0.0.1:3001"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With", "X-Workspace-Id", "X-Request-Id"},
		AllowCredentials: true,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","version":"1.0.0"}`))
	})

	public := responses.NewPublic(pool)
	authSvc := auth.New(pool, cfg.JWTSecret)

	r.Route("/api/v1", func(api chi.Router) {
		// public (no auth)
		api.Route("/public", func(pub chi.Router) {
			public.Routes(pub)
		})
		api.Post("/ai/chat", aiChat)
		api.Get("/search", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query().Get("q")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"query":"` + q + `","results":{"forms":[],"templates":[],"responses":[]}}`))
		})
		// auth public endpoints
		authSvc.Routes(api)

		// protected group
		api.Group(func(pr chi.Router) {
			if cfg.Env == "development" && devUserID != "" {
				pr.Use(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, devUserID)))
					})
				})
			} else {
				pr.Use(auth.RequireAuth(cfg.JWTSecret))
			}
			workspace.New(pool).Routes(pr)
			forms.New(pool).Routes(pr)
			responses.New(pool).Routes(pr)
			analytics.New(pool).Routes(pr)
			templates.New(pool).Routes(pr)
			webhooks.New(pool).Routes(pr)
			pr.Get("/activity", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[]`))
			})
			pr.Get("/notifications", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[]`))
			})
		})
	})

	port := cfg.Port
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on :%s env=%s", port, cfg.Env)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
		_ = os.Exit
	}
}

func ensureDevelopmentAccount(ctx context.Context, pool *pgxpool.Pool) string {
	var userID string
	err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name) VALUES('rishav@novacloud22.dev','development-only','Rishav Jha') ON CONFLICT(email) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`).Scan(&userID)
	if err != nil {
		log.Printf("development account unavailable: %v", err)
		return ""
	}
	var workspaceID string
	_ = pool.QueryRow(ctx, `INSERT INTO workspaces(name,slug,owner_id) VALUES('Rishav''s Workspace','rishav-workspace',$1) ON CONFLICT(slug) DO UPDATE SET owner_id=EXCLUDED.owner_id RETURNING id::text`, userID).Scan(&workspaceID)
	_, _ = pool.Exec(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'Owner') ON CONFLICT(workspace_id,user_id) DO NOTHING`, workspaceID, userID)
	log.Printf("development account ready: %s", userID)
	return userID
}

func aiChat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"reply":"Hi Rishav! I'm your Syntroform AI assistant. I can help you design surveys, configure adaptive follow-ups, optimize completion rates, or analyze responses.","suggestions":["Create a CSAT survey","Where is drop-off highest?","Summarize last 50 responses"]}`))
}
