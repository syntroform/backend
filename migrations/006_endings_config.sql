-- 006_endings_config: Configurable endings (End screen vs Redirect URL)
ALTER TABLE form_settings ADD COLUMN IF NOT EXISTS ending_type TEXT NOT NULL DEFAULT 'end_screen';
ALTER TABLE form_settings ADD COLUMN IF NOT EXISTS button_text TEXT NOT NULL DEFAULT 'Submit another response';
ALTER TABLE form_settings ADD COLUMN IF NOT EXISTS button_action TEXT NOT NULL DEFAULT 'restart';
ALTER TABLE form_settings ADD COLUMN IF NOT EXISTS button_url TEXT;
ALTER TABLE form_settings ADD COLUMN IF NOT EXISTS show_social_share BOOLEAN NOT NULL DEFAULT FALSE;
