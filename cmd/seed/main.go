package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"syntroform/backend/internal/config"
	"syntroform/backend/internal/db"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection error: %v", err)
	}
	defer pool.Close()

	fmt.Println("Connected to Neon Database:", cfg.DatabaseURL)

	// Ensure response_answers columns are TEXT to support all question IDs and Adaptive AI follow-ups
	_, _ = pool.Exec(ctx, `ALTER TABLE response_answers ALTER COLUMN question_id TYPE TEXT; ALTER TABLE response_answers ALTER COLUMN section_id TYPE TEXT;`)

	// 1. Ensure user rishav@novacloud22.dev
	var userID string
	err = pool.QueryRow(ctx, `
		INSERT INTO users(email, password_hash, name, email_verified)
		VALUES('rishav@novacloud22.dev', 'development-only', 'Rishav Jha', true)
		ON CONFLICT(email) DO UPDATE SET name=EXCLUDED.name
		RETURNING id::text
	`).Scan(&userID)
	if err != nil {
		log.Fatalf("Error inserting/getting user: %v", err)
	}
	fmt.Printf("✓ User ready: Rishav Jha (%s) [ID: %s]\n", "rishav@novacloud22.dev", userID)

	// 2. Ensure Workspace
	var workspaceID string
	err = pool.QueryRow(ctx, `
		INSERT INTO workspaces(name, slug, owner_id)
		VALUES('Rishav''s Workspace', 'rishav-workspace', $1)
		ON CONFLICT(slug) DO UPDATE SET owner_id=EXCLUDED.owner_id
		RETURNING id::text
	`, userID).Scan(&workspaceID)
	if err != nil {
		log.Fatalf("Error inserting/getting workspace: %v", err)
	}
	fmt.Printf("✓ Workspace ready: Rishav's Workspace [ID: %s]\n", workspaceID)

	// 3. Ensure membership
	_, err = pool.Exec(ctx, `
		INSERT INTO workspace_members(workspace_id, user_id, role)
		VALUES($1, $2, 'Owner')
		ON CONFLICT(workspace_id, user_id) DO UPDATE SET role='Owner'
	`, workspaceID, userID)
	if err != nil {
		log.Fatalf("Error setting workspace membership: %v", err)
	}

	// 4. Create/Upsert Customer Satisfaction Demo Form
	publicSlug := "customer-satisfaction"
	title := "Customer Experience Survey – Q1 2026"
	description := "Measure satisfaction across touchpoints and identify drivers of loyalty."

	headerJSON, _ := json.Marshal(map[string]any{
		"brandName":   "Syntroform",
		"title":       title,
		"description": description,
		"layout":      "text",
		"alignment":   "left",
		"background":  "#ffffff",
		"textColor":   "#0a0a0a",
		"accentColor": "#0a0a0a",
		"spacing":     "comfortable",
	})

	var formID string
	err = pool.QueryRow(ctx, `
		INSERT INTO forms(public_id, workspace_id, created_by, title, description, status, mode, header, updated_at)
		VALUES($1, $2, $3, $4, $5, 'published', 'flow', $6::jsonb, now())
		ON CONFLICT(public_id) DO UPDATE SET
			workspace_id=EXCLUDED.workspace_id,
			created_by=EXCLUDED.created_by,
			title=EXCLUDED.title,
			description=EXCLUDED.description,
			status='published',
			mode='flow',
			header=EXCLUDED.header,
			updated_at=now()
		RETURNING id::text
	`, publicSlug, workspaceID, userID, title, description, string(headerJSON)).Scan(&formID)
	if err != nil {
		log.Fatalf("Error inserting form: %v", err)
	}
	fmt.Printf("✓ Form ready in Neon DB: %s [ID: %s, Slug: /f/%s]\n", title, formID, publicSlug)

	// 5. Settings
	_, err = pool.Exec(ctx, `
		INSERT INTO form_settings(form_id, open, multiple_submissions, anonymous, save_continue, confirmation_title, confirmation_message, analytics_enabled)
		VALUES($1, true, false, false, true, 'Thank you — we''re listening.', 'Your feedback helps shape what we build next.', true)
		ON CONFLICT(form_id) DO UPDATE SET
			open=true,
			multiple_submissions=false,
			confirmation_title='Thank you — we''re listening.',
			confirmation_message='Your feedback helps shape what we build next.'
	`, formID)
	if err != nil {
		log.Printf("Warning setting form settings: %v", err)
	}

	// 6. Section
	var sectionID string
	err = pool.QueryRow(ctx, `
		INSERT INTO form_sections(form_id, title, description, position)
		VALUES($1, 'Customer Sentiment', 'Tell us about your recent experience.', 0)
		RETURNING id::text
	`, formID).Scan(&sectionID)
	if err != nil {
		// Try fetching existing section
		_ = pool.QueryRow(ctx, `SELECT id::text FROM form_sections WHERE form_id=$1 LIMIT 1`, formID).Scan(&sectionID)
	}

	// Clear out existing questions for this form so we cleanly re-seed
	_, _ = pool.Exec(ctx, `DELETE FROM form_questions WHERE form_id=$1`, formID)

	// 7. Seed Questions
	type qDef struct {
		Type        string
		Title       string
		Description string
		Placeholder string
		Required    bool
		Position    int
		Settings    map[string]any
		Options     []struct{ Label, Value string }
	}

	questions := []qDef{
		{
			Type:        "opinion_scale",
			Title:       "How would you rate your overall experience?",
			Description: "Your rating helps us improve our product quality.",
			Required:    true,
			Position:    0,
			Settings:    map[string]any{"scaleMin": 1, "scaleMax": 5, "scaleLabelLeft": "Dissatisfied", "scaleLabelRight": "Delighted"},
		},
		{
			Type:        "single_choice",
			Title:       "What best describes your relationship with us?",
			Description: "",
			Required:    false,
			Position:    1,
			Options: []struct{ Label, Value string }{
				{"First-time customer", "first"},
				{"Returning customer", "returning"},
				{"Partner / Vendor", "partner"},
				{"Just exploring", "exploring"},
			},
		},
		{
			Type:        "long_text",
			Title:       "What is one thing we could do to make your experience 10/10?",
			Description: "Share specific thoughts or friction points.",
			Placeholder: "Share your thoughts…",
			Required:    false,
			Position:    2,
		},
		{
			Type:        "multiple_choice",
			Title:       "Which areas matter most to you? (select up to 3)",
			Description: "",
			Required:    false,
			Position:    3,
			Settings:    map[string]any{"maxSelections": 3},
			Options: []struct{ Label, Value string }{
				{"Speed & performance", "speed"},
				{"Design quality & UX", "design"},
				{"Customer support", "support"},
				{"Pricing & value", "pricing"},
				{"Reliability & uptime", "reliability"},
			},
		},
		{
			Type:        "email",
			Title:       "Where should we send a follow-up or reward?",
			Description: "Optional contact email.",
			Placeholder: "you@company.com",
			Required:    false,
			Position:    4,
		},
	}

	for _, q := range questions {
		settingsJSON, _ := json.Marshal(q.Settings)
		if q.Settings == nil {
			settingsJSON = []byte("{}")
		}
		var qID string
		err = pool.QueryRow(ctx, `
			INSERT INTO form_questions(form_id, section_id, type, title, description, placeholder, required, position, settings)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
			RETURNING id::text
		`, formID, sectionID, q.Type, q.Title, q.Description, q.Placeholder, q.Required, q.Position, string(settingsJSON)).Scan(&qID)

		if err != nil {
			log.Printf("Error adding question %s: %v", q.Title, err)
			continue
		}

		for optIdx, opt := range q.Options {
			_, err = pool.Exec(ctx, `
				INSERT INTO question_options(question_id, label, value, position)
				VALUES($1, $2, $3, $4)
			`, qID, opt.Label, opt.Value, optIdx)
			if err != nil {
				log.Printf("Error adding option %s: %v", opt.Label, err)
			}
		}
	}

	// 8. Theme
	themeJSON, _ := json.Marshal(map[string]any{
		"background": "#FCFCF9",
		"card":       "#FFFFFF",
		"text":       "#0a0a0a",
		"accent":     "#0a0a0a",
		"muted":      "#f5f5f3",
		"border":     "#e8e8e3",
	})
	_, _ = pool.Exec(ctx, `
		INSERT INTO themes(form_id, preset, config)
		VALUES($1, 'minimal', $2::jsonb)
		ON CONFLICT(form_id) DO UPDATE SET preset='minimal', config=$2::jsonb
	`, formID, string(themeJSON))

	fmt.Println("\n=======================================================")
	fmt.Println("🚀 SEED COMPLETED DIRECTLY IN NEON POSTGRESQL DATABASE!")
	fmt.Printf("User: rishav@novacloud22.dev (Rishav Jha)\n")
	fmt.Printf("Form: %s\n", title)
	fmt.Printf("Public URL: /f/%s\n", publicSlug)
	fmt.Printf("Builder URL: /builder/%s\n", formID)
	fmt.Println("=======================================================")
}
