-- 005_welcome_screen: configurable welcome screen metadata
ALTER TABLE forms ADD COLUMN IF NOT EXISTS welcome_screen JSONB NOT NULL DEFAULT '{}'::jsonb;
