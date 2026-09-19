package jobs

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Start(ctx context.Context, pool *pgxpool.Pool) {
	go analyticsAggregator(ctx, pool)
	go cleanup(ctx, pool)
}

func analyticsAggregator(ctx context.Context, pool *pgxpool.Pool) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// aggregate daily stats from analytics_events where not yet aggregated
			_, err := pool.Exec(ctx, `
				INSERT INTO form_daily_stats(form_id, day, views, starts, completions)
				SELECT form_id, CURRENT_DATE, 
				       COUNT(*) FILTER (WHERE event_type='form_view'),
				       COUNT(*) FILTER (WHERE event_type='form_start'),
				       COUNT(*) FILTER (WHERE event_type='form_complete')
				FROM analytics_events WHERE created_at >= CURRENT_DATE
				GROUP BY form_id
				ON CONFLICT(form_id, day) DO UPDATE SET views=EXCLUDED.views, starts=EXCLUDED.starts, completions=EXCLUDED.completions
			`)
			if err != nil {
				log.Printf("jobs: aggregator %v", err)
			}
		}
	}
}

func cleanup(ctx context.Context, pool *pgxpool.Pool) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = pool.Exec(ctx, `DELETE FROM analytics_events WHERE created_at < now() - interval '365 days'`)
		}
	}
}
