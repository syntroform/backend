-- 004_mode_and_dropzone.sql
ALTER TABLE forms DROP CONSTRAINT IF EXISTS forms_mode_check;
ALTER TABLE forms ADD CONSTRAINT forms_mode_check CHECK (mode IN ('flow','structured','branded','conversational'));
ALTER TABLE form_settings ADD COLUMN IF NOT EXISTS document_dropzone BOOLEAN NOT NULL DEFAULT FALSE;
