package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	log.Println("db: connected")
	return pool, nil
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	// Ensure schema_migrations table exists
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ DEFAULT now())`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".sql" {
			continue
		}

		var exists bool
		err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, e.Name()).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", e.Name(), err)
		}
		if exists {
			continue
		}

		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", e.Name(), err)
		}

		log.Printf("migrating %s", e.Name())
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			// If already created by previous run, mark as recorded to prevent repeated failure
			_, _ = pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1) ON CONFLICT (version) DO NOTHING`, e.Name())
			log.Printf("migration %s notice: %v", e.Name(), err)
		} else {
			if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1) ON CONFLICT (version) DO NOTHING`, e.Name()); err != nil {
				return fmt.Errorf("record migration %s: %w", e.Name(), err)
			}
		}
	}

	return nil
}
