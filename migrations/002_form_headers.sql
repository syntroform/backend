-- 002_form_headers: configurable public form header metadata
ALTER TABLE forms ADD COLUMN IF NOT EXISTS header JSONB NOT NULL DEFAULT '{}'::jsonb;
