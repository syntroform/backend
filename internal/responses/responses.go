package responses

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"syntroform/backend/internal/httpx"
)

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Routes(r chi.Router) {
	r.Get("/responses", s.listAll)
	r.Get("/forms/{id}/responses", s.list)
	r.Get("/responses/{id}", s.get)
	r.Patch("/responses/{id}", s.update)
	r.Delete("/responses/{id}", s.delete)
}

func (s *Service) listAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, `
		SELECT r.id::text, r.form_id::text, r.public_id, COALESCE(r.form_version_id::text, ''),
		       COALESCE(r.status, 'completed'), COALESCE(r.source, 'direct'), COALESCE(r.device, 'desktop'),
		       COALESCE(r.started_at, now()), COALESCE(r.submitted_at, now()), COALESCE(r.completion_sec, 0),
		       COALESCE(f.title, 'Untitled form'),
		       COALESCE((
		           SELECT ra.value::text FROM response_answers ra 
		           JOIN form_questions fq ON fq.id::text = ra.question_id::text 
		           WHERE ra.response_id = r.id AND (fq.type = 'email' OR LOWER(fq.title) LIKE '%email%')
		           LIMIT 1
		       ), ''),
		       COALESCE((
		           SELECT ra.value::text FROM response_answers ra 
		           JOIN form_questions fq ON fq.id::text = ra.question_id::text 
		           WHERE ra.response_id = r.id AND (fq.type = 'name' OR LOWER(fq.title) LIKE '%name%')
		           LIMIT 1
		       ), '')
		FROM responses r
		LEFT JOIN forms f ON f.id = r.form_id
		ORDER BY r.submitted_at DESC
		LIMIT 200`)
	if err != nil {
		log.Printf("[responses:listAll] query error: %v", err)
		httpx.JSON(w, 200, []any{})
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, fid, pub, ver, status, source, device, formTitle, emailVal, nameVal string
		var started, submitted time.Time
		var comp int
		if err := rows.Scan(&id, &fid, &pub, &ver, &status, &source, &device, &started, &submitted, &comp, &formTitle, &emailVal, &nameVal); err != nil {
			continue
		}
		emailClean := strings.Trim(emailVal, `"`)
		nameClean := strings.Trim(nameVal, `"`)
		if nameClean == "" && emailClean != "" {
			parts := strings.Split(emailClean, "@")
			nameClean = parts[0]
		}
		if nameClean == "" {
			nameClean = "Respondent " + pub
		}

		out = append(out, map[string]any{
			"id":              id,
			"form_id":         fid,
			"form_title":      formTitle,
			"public_id":       pub,
			"form_version_id": ver,
			"status":          status,
			"source":          source,
			"device":          device,
			"started_at":      started,
			"submitted_at":    submitted,
			"completion_sec":  comp,
			"name":            nameClean,
			"email":           emailClean,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	httpx.JSON(w, 200, out)
}

// list creator responses
func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	formID := chi.URLParam(r, "id")
	rows, err := s.pool.Query(r.Context(), `
		SELECT r.id::text, r.form_id::text, r.public_id, COALESCE(r.form_version_id::text, ''),
		       COALESCE(r.status, 'completed'), COALESCE(r.source, 'direct'), COALESCE(r.device, 'desktop'),
		       COALESCE(r.browser, 'Safari'), COALESCE(r.os, 'macOS'), COALESCE(r.referrer, 'Direct'),
		       COALESCE(r.started_at, now()), COALESCE(r.submitted_at, now()), COALESCE(r.completion_sec, 0),
		       COALESCE((
		           SELECT ra.value::text FROM response_answers ra 
		           JOIN form_questions fq ON fq.id::text = ra.question_id::text 
		           WHERE ra.response_id = r.id AND (fq.type = 'email' OR LOWER(fq.title) LIKE '%email%')
		           LIMIT 1
		       ), ''),
		       COALESCE((
		           SELECT ra.value::text FROM response_answers ra 
		           JOIN form_questions fq ON fq.id::text = ra.question_id::text 
		           WHERE ra.response_id = r.id AND (fq.type = 'name' OR LOWER(fq.title) LIKE '%name%')
		           LIMIT 1
		       ), ''),
		       COALESCE((
		           SELECT jsonb_agg(jsonb_build_object('question_id', ra.question_id::text, 'value', ra.value))
		           FROM response_answers ra
		           WHERE ra.response_id = r.id
		       ), '[]'::jsonb)
		FROM responses r
		WHERE r.form_id::text = $1
		ORDER BY r.submitted_at DESC
		LIMIT 100`, formID)
	if err != nil {
		log.Printf("[responses:list] query error for formID=%s: %v", formID, err)
		httpx.JSON(w, 200, []any{})
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, fid, pub, ver, status, source, device, browser, osVal, referrer, emailVal, nameVal string
		var started, submitted time.Time
		var comp int
		var answersRaw []byte
		if err := rows.Scan(&id, &fid, &pub, &ver, &status, &source, &device, &browser, &osVal, &referrer, &started, &submitted, &comp, &emailVal, &nameVal, &answersRaw); err != nil {
			continue
		}
		emailClean := strings.Trim(emailVal, `"`)
		nameClean := strings.Trim(nameVal, `"`)
		if nameClean == "" && emailClean != "" {
			parts := strings.Split(emailClean, "@")
			nameClean = parts[0]
		}
		if nameClean == "" {
			nameClean = "Respondent " + pub
		}

		var answersList []any
		if len(answersRaw) > 0 {
			_ = json.Unmarshal(answersRaw, &answersList)
		}
		if answersList == nil {
			answersList = []any{}
		}

		out = append(out, map[string]any{
			"id":              id,
			"form_id":         fid,
			"public_id":       pub,
			"form_version_id": ver,
			"status":          status,
			"source":          source,
			"device":          device,
			"browser":         browser,
			"os":              osVal,
			"referrer":        referrer,
			"started_at":      started,
			"submitted_at":    submitted,
			"completion_sec":  comp,
			"name":            nameClean,
			"email":           emailClean,
			"answers":         answersList,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	httpx.JSON(w, 200, out)
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	var payload []byte
	err := s.pool.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'response', jsonb_build_object(
				'id', r.id::text,
				'form_id', r.form_id::text,
				'public_id', r.public_id,
				'form_version_id', COALESCE(r.form_version_id::text, ''),
				'status', COALESCE(r.status, 'completed'),
				'source', COALESCE(r.source, 'direct'),
				'device', COALESCE(r.device, 'desktop'),
				'browser', COALESCE(r.browser, 'Safari'),
				'os', COALESCE(r.os, 'macOS'),
				'referrer', COALESCE(r.referrer, 'Direct'),
				'started_at', COALESCE(r.started_at, now()),
				'submitted_at', COALESCE(r.submitted_at, now()),
				'completion_sec', COALESCE(r.completion_sec, 0),
				'workspace_id', COALESCE(r.workspace_id::text, '')
			),
			'answers', COALESCE((
				SELECT jsonb_agg(jsonb_build_object('question_id', ra.question_id::text, 'value', ra.value))
				FROM response_answers ra
				WHERE ra.response_id = r.id
			), '[]'::jsonb),
			'form', (
				SELECT jsonb_build_object(
					'id', f.id::text,
					'title', f.title,
					'description', f.description,
					'questions', COALESCE((
						SELECT jsonb_agg(
							to_jsonb(q) || jsonb_build_object(
								'id', q.id::text,
								'section_id', COALESCE(q.section_id::text, ''),
								'options', COALESCE((
									SELECT jsonb_agg(to_jsonb(o) ORDER BY o.position)
									FROM question_options o
									WHERE o.question_id = q.id
								), '[]'::jsonb)
							)
							ORDER BY q.position
						)
						FROM form_questions q
						WHERE q.form_id = f.id
					), '[]'::jsonb)
				)
				FROM forms f
				WHERE f.id = r.form_id
			)
		)
		FROM responses r
		WHERE r.id::text = $1 OR r.public_id = $1
		LIMIT 1`, id).Scan(&payload)

	if err != nil {
		log.Printf("[responses:get] response not found for id=%s: %v", id, err)
		httpx.Error(w, 404, "NOT_FOUND", "response not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(payload)
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	if v, ok := req["status"].(string); ok {
		_, _ = s.pool.Exec(r.Context(), `UPDATE responses SET status=$1 WHERE id=$2`, v, id)
	}
	httpx.JSON(w, 200, map[string]any{"ok": true})
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, _ = s.pool.Exec(r.Context(), `DELETE FROM responses WHERE id::text=$1 OR public_id=$1`, id)
	httpx.JSON(w, 200, map[string]any{"ok": true})
}

// Public submission
type PublicService struct{ pool *pgxpool.Pool }

func NewPublic(pool *pgxpool.Pool) *PublicService { return &PublicService{pool: pool} }

func (p *PublicService) Routes(r chi.Router) {
	r.Get("/forms/{slug}", p.getForm)
	r.Post("/forms/{slug}/events", p.event)
	r.Post("/forms/{slug}/submit", p.submit)
	r.Get("/forms/{slug}/submit", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"status": "ok", "message": "Submit endpoint is active. Use POST to submit form answers."})
	})
	r.Get("/public/forms/{slug}", p.getForm)
	r.Post("/public/forms/{slug}/events", p.event)
	r.Post("/public/forms/{slug}/submit", p.submit)
}

func (p *PublicService) getForm(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var id, title, desc, mode, status string
	var header, welcomeScreen []byte
	var snapshot []byte
	var versionID string
	err := p.pool.QueryRow(r.Context(), `
		SELECT f.id::text, f.title, f.description, f.mode, f.status, COALESCE(f.header, '{}'::jsonb), COALESCE(f.welcome_screen, '{}'::jsonb), COALESCE(v.id::text, ''), COALESCE(v.snapshot, '{}'::jsonb)
		FROM forms f
		LEFT JOIN LATERAL (SELECT id, snapshot FROM form_versions WHERE form_id=f.id ORDER BY version_no DESC LIMIT 1) v ON true
		WHERE (
			f.public_id=$1 OR 
			f.id::text=$1 OR 
			LOWER(f.public_id)=LOWER($1) OR 
			LOWER(REPLACE(f.title, ' ', '-'))=LOWER($1) OR 
			LOWER(f.title)=LOWER($1) OR 
			f.public_id ILIKE $1 || '%'
		)
		ORDER BY (f.public_id=$1) DESC, f.updated_at DESC
		LIMIT 1`, slug).Scan(&id, &title, &desc, &mode, &status, &header, &welcomeScreen, &versionID, &snapshot)
	if err != nil {
		httpx.Error(w, 404, "NOT_FOUND", "form not found")
		return
	}
	if status == "closed" || status == "archived" {
		httpx.Error(w, 410, "FORM_CLOSED", "form is closed")
		return
	}
	if len(snapshot) > 0 && string(snapshot) != "{}" && string(snapshot) != "null" {
		var published map[string]any
		if json.Unmarshal(snapshot, &published) == nil {
			if qs, ok := published["questions"].([]any); ok && len(qs) > 0 {
				if versionID != "" {
					published["form_version_id"] = versionID
				}
				if _, hasWS := published["welcome_screen"]; !hasWS && len(welcomeScreen) > 0 {
					published["welcome_screen"] = json.RawMessage(welcomeScreen)
					published["welcomeScreen"] = json.RawMessage(welcomeScreen)
				}
				httpx.JSON(w, 200, published)
				return
			}
		}
	}
	// load settings
	var confirmationTitle, confirmationMessage, redirectURL string
	var settingsMap map[string]any
	_ = p.pool.QueryRow(r.Context(), `SELECT to_jsonb(form_settings) FROM form_settings WHERE form_id=$1`, id).Scan(&settingsMap)
	if settingsMap == nil {
		settingsMap = map[string]any{}
	}
	_ = p.pool.QueryRow(r.Context(), `SELECT confirmation_title,confirmation_message,COALESCE(redirect_url,'') FROM form_settings WHERE form_id=$1`, id).Scan(&confirmationTitle, &confirmationMessage, &redirectURL)
	// sections
	srows, _ := p.pool.Query(r.Context(), `SELECT id::text,title,description,position FROM form_sections WHERE form_id=$1 ORDER BY position`, id)
	var sections []map[string]any
	for srows.Next() {
		var sid, stitle, sdesc string
		var pos int
		_ = srows.Scan(&sid, &stitle, &sdesc, &pos)
		sections = append(sections, map[string]any{"id": sid, "title": stitle, "description": sdesc, "position": pos})
	}
	srows.Close()
	qrows, _ := p.pool.Query(r.Context(), `SELECT id::text,COALESCE(section_id::text,''),type,title,description,placeholder,help_text,required,position,validation,settings FROM form_questions WHERE form_id=$1 ORDER BY position`, id)
	var questions []map[string]any
	for qrows.Next() {
		var qid, secID, qtype, qtitle, qdesc, placeholder, help string
		var required bool
		var pos int
		var validation, settings []byte
		_ = qrows.Scan(&qid, &secID, &qtype, &qtitle, &qdesc, &placeholder, &help, &required, &pos, &validation, &settings)
		orows, _ := p.pool.Query(r.Context(), `SELECT label,value FROM question_options WHERE question_id=$1 ORDER BY position`, qid)
		var opts []map[string]any
		for orows.Next() {
			var lab, val string
			_ = orows.Scan(&lab, &val)
			opts = append(opts, map[string]any{"label": lab, "value": val})
		}
		orows.Close()
		questions = append(questions, map[string]any{"id": qid, "section_id": secID, "type": qtype, "title": qtitle, "description": qdesc, "placeholder": placeholder, "help_text": help, "required": required, "position": pos, "validation": json.RawMessage(validation), "settings": json.RawMessage(settings), "options": opts})
	}
	qrows.Close()
	if sections == nil {
		sections = []map[string]any{}
	}
	if questions == nil {
		questions = []map[string]any{}
	}
	// theme
	var preset string
	var cfg []byte
	_ = p.pool.QueryRow(r.Context(), `SELECT preset,config FROM themes WHERE form_id=$1`, id).Scan(&preset, &cfg)
	httpx.JSON(w, 200, map[string]any{
		"id": id, "public_id": slug, "title": title, "description": desc, "mode": mode, "status": status,
		"sections": sections, "questions": questions, "settings": settingsMap,
		"header": json.RawMessage(header),
		"welcome_screen": json.RawMessage(welcomeScreen),
		"welcomeScreen": json.RawMessage(welcomeScreen),
		"theme": map[string]any{"preset": preset, "config": json.RawMessage(cfg)},
		"confirmation_title": confirmationTitle, "confirmation_message": confirmationMessage, "redirect_url": redirectURL,
	})
}

func (p *PublicService) event(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var formID string
	_ = p.pool.QueryRow(r.Context(), `
		SELECT id::text FROM forms
		WHERE (
			public_id=$1 OR 
			id::text=$1 OR 
			LOWER(public_id)=LOWER($1) OR 
			LOWER(REPLACE(title, ' ', '-'))=LOWER($1) OR 
			LOWER(title)=LOWER($1) OR 
			public_id ILIKE $1 || '%'
		)
		ORDER BY (public_id=$1) DESC, updated_at DESC
		LIMIT 1`, slug).Scan(&formID)
	if formID == "" {
		httpx.Error(w, 404, "NOT_FOUND", "form not found")
		return
	}
	var req struct {
		SessionID  string         `json:"session_id"`
		Type       string         `json:"type"`
		QuestionID *string        `json:"question_id"`
		SectionID  *string        `json:"section_id"`
		Device     string         `json:"device"`
		Source     string         `json:"source"`
		Meta       map[string]any `json:"meta"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Type == "" {
		req.Type = "form_view"
	}
	if req.SessionID == "" {
		req.SessionID = uuid.NewString()
	}
	var qid, sid any
	if req.QuestionID != nil {
		qid = *req.QuestionID
	}
	if req.SectionID != nil {
		sid = *req.SectionID
	}
	meta, _ := json.Marshal(req.Meta)
	_, _ = p.pool.Exec(r.Context(), `INSERT INTO analytics_events(session_id,form_id,event_type,question_id,section_id,device,source,meta) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, req.SessionID, formID, req.Type, qid, sid, req.Device, req.Source, meta)
	httpx.JSON(w, 200, map[string]any{"ok": true, "session_id": req.SessionID})
}

func (p *PublicService) submit(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	log.Printf("[public:submit] processing submission for slug=%s from IP=%s", slug, r.RemoteAddr)
	var formID, workspaceID, status string
	err := p.pool.QueryRow(r.Context(), `
		SELECT id::text,workspace_id::text,status FROM forms
		WHERE (
			public_id=$1 OR 
			id::text=$1 OR 
			LOWER(public_id)=LOWER($1) OR 
			LOWER(REPLACE(title, ' ', '-'))=LOWER($1) OR 
			LOWER(title)=LOWER($1) OR 
			public_id ILIKE $1 || '%'
		)
		ORDER BY (public_id=$1) DESC, updated_at DESC
		LIMIT 1`, slug).Scan(&formID, &workspaceID, &status)
	if err != nil {
		log.Printf("[public:submit] form not found for slug=%s: %v", slug, err)
		httpx.Error(w, 404, "NOT_FOUND", "form not found")
		return
	}
	if status == "closed" || status == "archived" {
		log.Printf("[public:submit] form closed for slug=%s status=%s", slug, status)
		httpx.Error(w, 410, "FORM_CLOSED", "form closed")
		return
	}
	var req struct {
		Answers []struct {
			QuestionID string `json:"question_id"`
			Value      any    `json:"value"`
		} `json:"answers"`
		SessionID string     `json:"session_id"`
		Source    string     `json:"source"`
		Device    string     `json:"device"`
		Browser   string     `json:"browser"`
		OS        string     `json:"os"`
		Referrer  string     `json:"referrer"`
		StartedAt *time.Time `json:"started_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[public:submit] bad JSON body: %v", err)
		httpx.Error(w, 400, "BAD_REQUEST", "invalid body")
		return
	}
	log.Printf("[public:submit] formID=%s workspaceID=%s answers_count=%d", formID, workspaceID, len(req.Answers))

	// Detect metadata if not provided
	ua := r.UserAgent()
	if req.OS == "" {
		if strings.Contains(ua, "Macintosh") || strings.Contains(ua, "Mac OS") {
			req.OS = "macOS"
		} else if strings.Contains(ua, "Windows") {
			req.OS = "Windows"
		} else if strings.Contains(ua, "Android") {
			req.OS = "Android"
		} else if strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad") || strings.Contains(ua, "iOS") {
			req.OS = "iOS"
		} else if strings.Contains(ua, "Linux") {
			req.OS = "Linux"
		} else {
			req.OS = "macOS"
		}
	}
	if req.Browser == "" {
		if strings.Contains(ua, "Edg/") {
			req.Browser = "Edge"
		} else if strings.Contains(ua, "Chrome/") {
			req.Browser = "Chrome"
		} else if strings.Contains(ua, "Safari/") {
			req.Browser = "Safari"
		} else if strings.Contains(ua, "Firefox/") {
			req.Browser = "Firefox"
		} else {
			req.Browser = "Safari"
		}
	}
	if req.Referrer == "" {
		ref := r.Referer()
		if ref == "" || strings.Contains(ref, "localhost") {
			req.Referrer = "Direct"
		} else if strings.Contains(ref, "google.") {
			req.Referrer = "Google"
		} else if strings.Contains(ref, "twitter.") || strings.Contains(ref, "t.co") || strings.Contains(ref, "x.com") {
			req.Referrer = "Twitter / X"
		} else if strings.Contains(ref, "linkedin.") {
			req.Referrer = "LinkedIn"
		} else {
			req.Referrer = "Direct"
		}
	}
	if req.Source == "" {
		req.Source = "direct"
	}
	if req.Device == "" {
		if strings.Contains(ua, "Mobile") || strings.Contains(ua, "Android") || strings.Contains(ua, "iPhone") {
			req.Device = "mobile"
		} else {
			req.Device = "desktop"
		}
	}

	// find published version if exists
	var versionID *string
	_ = p.pool.QueryRow(r.Context(), `SELECT id::text FROM form_versions WHERE form_id=$1 ORDER BY version_no DESC LIMIT 1`, formID).Scan(&versionID)
	var ver any
	if versionID != nil && *versionID != "" {
		ver = *versionID
	}
	pubID := uuid.NewString()[:8]
	started := time.Now().Add(-30 * time.Second)
	if req.StartedAt != nil {
		started = *req.StartedAt
	}
	compSec := int(time.Since(started).Seconds())
	if compSec < 0 {
		compSec = 0
	}
	var respID string
	tx, err := p.pool.Begin(r.Context())
	if err != nil {
		log.Printf("[public:submit] tx error: %v", err)
		httpx.Error(w, 500, "INTERNAL", "tx")
		return
	}
	defer tx.Rollback(r.Context())
	err = tx.QueryRow(r.Context(), `
		INSERT INTO responses(public_id,form_id,form_version_id,workspace_id,source,device,browser,os,referrer,started_at,submitted_at,completion_sec)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now(),$11) RETURNING id::text`,
		pubID, formID, ver, workspaceID, req.Source, req.Device, req.Browser, req.OS, req.Referrer, started, compSec).Scan(&respID)
	if err != nil {
		log.Printf("[public:submit] INSERT responses error: %v", err)
		httpx.Error(w, 500, "INTERNAL", err.Error())
		return
	}
	var b pgx.Batch
	for _, a := range req.Answers {
		val, _ := json.Marshal(a.Value)
		b.Queue(`
			INSERT INTO response_answers(response_id,question_id,value)
			VALUES($1,$2,$3)`, respID, a.QuestionID, val)
	}
	if req.SessionID != "" {
		b.Queue(`INSERT INTO analytics_events(session_id,form_id,event_type,device,source) VALUES($1,$2,'form_complete',$3,$4)`, req.SessionID, formID, req.Device, req.Source)
	}
	br := tx.SendBatch(r.Context(), &b)
	_ = br.Close()

	if cErr := tx.Commit(r.Context()); cErr != nil {
		log.Printf("[public:submit] commit error: %v", cErr)
		httpx.Error(w, 500, "INTERNAL", "commit")
		return
	}
	log.Printf("[public:submit] SUCCESS: created response id=%s public_id=%s for form=%s", respID, pubID, slug)
	go func() {
		_, _ = p.pool.Exec(context.Background(), `
			INSERT INTO form_daily_stats(form_id,day,completions)
			VALUES($1, CURRENT_DATE, 1)
			ON CONFLICT(form_id,day) DO UPDATE SET completions=form_daily_stats.completions+1`, formID)
	}()
	httpx.JSON(w, 201, map[string]any{"id": respID, "public_id": pubID, "ok": true})
}

// dummy import alias
var _ = pgx.ErrNoRows
