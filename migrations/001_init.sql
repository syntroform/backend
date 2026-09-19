-- 001_init: core schema
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- users
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  name TEXT NOT NULL,
  avatar_url TEXT,
  email_verified BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- workspaces
CREATE TABLE IF NOT EXISTS workspaces (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- workspace_members
CREATE TABLE IF NOT EXISTS workspace_members (
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('Owner','Admin','Editor','Viewer')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_workspace_members_user ON workspace_members(user_id);

-- forms
CREATE TABLE IF NOT EXISTS forms (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  public_id TEXT NOT NULL UNIQUE, -- short public slug for /f/{slug}
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  created_by UUID NOT NULL REFERENCES users(id),
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('draft','published','closed','archived')) DEFAULT 'draft',
  mode TEXT NOT NULL CHECK (mode IN ('flow','structured')) DEFAULT 'flow',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_forms_workspace ON forms(workspace_id);
CREATE INDEX IF NOT EXISTS idx_forms_status ON forms(status);
CREATE INDEX IF NOT EXISTS idx_forms_updated ON forms(updated_at DESC);

-- form_versions (immutable published snapshots)
CREATE TABLE IF NOT EXISTS form_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  version_no INT NOT NULL,
  snapshot JSONB NOT NULL, -- full form snapshot for audit
  created_by UUID REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(form_id, version_no)
);
CREATE INDEX IF NOT EXISTS idx_form_versions_form ON form_versions(form_id);

-- form_settings
CREATE TABLE IF NOT EXISTS form_settings (
  form_id UUID PRIMARY KEY REFERENCES forms(id) ON DELETE CASCADE,
  open BOOLEAN NOT NULL DEFAULT TRUE,
  start_at TIMESTAMPTZ,
  end_at TIMESTAMPTZ,
  response_limit INT,
  multiple_submissions BOOLEAN NOT NULL DEFAULT FALSE,
  anonymous BOOLEAN NOT NULL DEFAULT FALSE,
  allow_edit BOOLEAN NOT NULL DEFAULT FALSE,
  save_continue BOOLEAN NOT NULL DEFAULT TRUE,
  password_hash TEXT,
  confirmation_title TEXT NOT NULL DEFAULT 'Thank you!',
  confirmation_message TEXT NOT NULL DEFAULT 'Your response has been recorded.',
  redirect_url TEXT,
  analytics_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- themes
CREATE TABLE IF NOT EXISTS themes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  preset TEXT NOT NULL DEFAULT 'minimal',
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(form_id)
);

-- form_sections (first-class)
CREATE TABLE IF NOT EXISTS form_sections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  position INT NOT NULL,
  logic JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sections_form ON form_sections(form_id);
CREATE INDEX IF NOT EXISTS idx_sections_form_position ON form_sections(form_id, position);

-- form_questions
CREATE TABLE IF NOT EXISTS form_questions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  section_id UUID REFERENCES form_sections(id) ON DELETE SET NULL,
  type TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  placeholder TEXT NOT NULL DEFAULT '',
  help_text TEXT NOT NULL DEFAULT '',
  required BOOLEAN NOT NULL DEFAULT FALSE,
  position INT NOT NULL,
  validation JSONB NOT NULL DEFAULT '{}'::jsonb,
  settings JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_questions_form ON form_questions(form_id);
CREATE INDEX IF NOT EXISTS idx_questions_section ON form_questions(section_id);
CREATE INDEX IF NOT EXISTS idx_questions_form_position ON form_questions(form_id, position);

-- question_options
CREATE TABLE IF NOT EXISTS question_options (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  question_id UUID NOT NULL REFERENCES form_questions(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  value TEXT NOT NULL,
  position INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_options_question ON question_options(question_id);
CREATE INDEX IF NOT EXISTS idx_options_q_pos ON question_options(question_id, position);

-- logic_rules
CREATE TABLE IF NOT EXISTS logic_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  trigger_question_id UUID REFERENCES form_questions(id) ON DELETE CASCADE,
  operator TEXT NOT NULL DEFAULT 'AND',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_logic_form ON logic_rules(form_id);

CREATE TABLE IF NOT EXISTS logic_conditions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  rule_id UUID NOT NULL REFERENCES logic_rules(id) ON DELETE CASCADE,
  question_id UUID NOT NULL REFERENCES form_questions(id) ON DELETE CASCADE,
  op TEXT NOT NULL CHECK (op IN ('equals','not_equals','contains','not_contains','starts_with','ends_with','gt','lt','gte','lte','is_answered','is_not_answered')),
  value TEXT NOT NULL DEFAULT '',
  position INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_conditions_rule ON logic_conditions(rule_id);

CREATE TABLE IF NOT EXISTS logic_actions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  rule_id UUID NOT NULL REFERENCES logic_rules(id) ON DELETE CASCADE,
  action TEXT NOT NULL CHECK (action IN ('show_question','hide_question','skip_question','jump_section','jump_question','skip_section','end_form','set_value','calculate')),
  target_id TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_actions_rule ON logic_actions(rule_id);

-- responses
CREATE TABLE IF NOT EXISTS responses (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  public_id TEXT NOT NULL UNIQUE,
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  form_version_id UUID REFERENCES form_versions(id),
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('completed','partial','spam')) DEFAULT 'completed',
  source TEXT NOT NULL DEFAULT 'direct',
  device TEXT NOT NULL DEFAULT 'desktop',
  browser TEXT NOT NULL DEFAULT '',
  os TEXT NOT NULL DEFAULT '',
  referrer TEXT NOT NULL DEFAULT '',
  utm JSONB NOT NULL DEFAULT '{}'::jsonb,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completion_sec INT NOT NULL DEFAULT 0,
  geo JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_responses_form ON responses(form_id);
CREATE INDEX IF NOT EXISTS idx_responses_workspace ON responses(workspace_id);
CREATE INDEX IF NOT EXISTS idx_responses_form_version ON responses(form_version_id);
CREATE INDEX IF NOT EXISTS idx_responses_created ON responses(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_responses_status ON responses(status);

-- response_answers
CREATE TABLE IF NOT EXISTS response_answers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  response_id UUID NOT NULL REFERENCES responses(id) ON DELETE CASCADE,
  question_id UUID NOT NULL,
  section_id UUID,
  value JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_answers_response ON response_answers(response_id);
CREATE INDEX IF NOT EXISTS idx_answers_question ON response_answers(question_id);

-- analytics_events (raw)
CREATE TABLE IF NOT EXISTS analytics_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id TEXT NOT NULL,
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  form_version_id UUID REFERENCES form_versions(id),
  event_type TEXT NOT NULL CHECK (event_type IN ('form_view','form_start','section_view','question_view','question_answer','question_skip','question_error','question_back','form_complete','form_abandon','form_exit')),
  question_id UUID,
  section_id UUID,
  device TEXT NOT NULL DEFAULT 'desktop',
  source TEXT NOT NULL DEFAULT 'direct',
  country TEXT,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_events_form ON analytics_events(form_id);
CREATE INDEX IF NOT EXISTS idx_events_form_type ON analytics_events(form_id, event_type);
CREATE INDEX IF NOT EXISTS idx_events_session ON analytics_events(session_id);
CREATE INDEX IF NOT EXISTS idx_events_question ON analytics_events(question_id);
CREATE INDEX IF NOT EXISTS idx_events_created ON analytics_events(created_at DESC);

-- aggregates (materialized by worker)
CREATE TABLE IF NOT EXISTS form_daily_stats (
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  day DATE NOT NULL,
  views INT NOT NULL DEFAULT 0,
  starts INT NOT NULL DEFAULT 0,
  completions INT NOT NULL DEFAULT 0,
  PRIMARY KEY (form_id, day)
);

CREATE TABLE IF NOT EXISTS form_question_stats (
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  question_id UUID NOT NULL,
  views INT NOT NULL DEFAULT 0,
  answers INT NOT NULL DEFAULT 0,
  skips INT NOT NULL DEFAULT 0,
  dropoffs INT NOT NULL DEFAULT 0,
  avg_time_ms INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (form_id, question_id)
);

-- templates
CREATE TABLE IF NOT EXISTS templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  category TEXT NOT NULL,
  est_minutes INT NOT NULL DEFAULT 3,
  snapshot JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_templates_category ON templates(category);

-- webhooks
CREATE TABLE IF NOT EXISTS webhooks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  form_id UUID REFERENCES forms(id) ON DELETE CASCADE,
  url TEXT NOT NULL,
  secret TEXT NOT NULL,
  events TEXT[] NOT NULL DEFAULT ARRAY['response.completed'],
  active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_webhooks_workspace ON webhooks(workspace_id);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_deliveries_webhook ON webhook_deliveries(webhook_id);
CREATE INDEX IF NOT EXISTS idx_deliveries_status ON webhook_deliveries(status);

-- api_keys
CREATE TABLE IF NOT EXISTS api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  prefix TEXT NOT NULL,
  hash TEXT NOT NULL,
  created_by UUID REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_apikeys_workspace ON api_keys(workspace_id);

-- comments, activity_logs, notifications, tags
CREATE TABLE IF NOT EXISTS comments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  question_id UUID REFERENCES form_questions(id) ON DELETE CASCADE,
  author_id UUID NOT NULL REFERENCES users(id),
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_comments_form ON comments(form_id);

CREATE TABLE IF NOT EXISTS activity_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  form_id UUID REFERENCES forms(id) ON DELETE SET NULL,
  user_id UUID REFERENCES users(id),
  action TEXT NOT NULL,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_activity_workspace ON activity_logs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_activity_form ON activity_logs(form_id);
CREATE INDEX IF NOT EXISTS idx_activity_created ON activity_logs(created_at DESC);

CREATE TABLE IF NOT EXISTS notifications (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  body TEXT NOT NULL,
  read BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id);

-- response organization and analytics dimensions
CREATE TABLE IF NOT EXISTS tags (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  color TEXT NOT NULL DEFAULT '#0a0a0a',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, name)
);
CREATE INDEX IF NOT EXISTS idx_tags_workspace ON tags(workspace_id);

CREATE TABLE IF NOT EXISTS response_tags (
  response_id UUID NOT NULL REFERENCES responses(id) ON DELETE CASCADE,
  tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (response_id, tag_id)
);

CREATE TABLE IF NOT EXISTS analytics_sessions (
  id TEXT PRIMARY KEY,
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  device TEXT NOT NULL DEFAULT 'desktop',
  source TEXT NOT NULL DEFAULT 'direct',
  referrer TEXT NOT NULL DEFAULT '',
  utm JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_analytics_sessions_form ON analytics_sessions(form_id);

CREATE TABLE IF NOT EXISTS form_source_stats (
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  source TEXT NOT NULL,
  views INT NOT NULL DEFAULT 0,
  starts INT NOT NULL DEFAULT 0,
  completions INT NOT NULL DEFAULT 0,
  PRIMARY KEY (form_id, source)
);

CREATE TABLE IF NOT EXISTS form_device_stats (
  form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  device TEXT NOT NULL,
  views INT NOT NULL DEFAULT 0,
  starts INT NOT NULL DEFAULT 0,
  completions INT NOT NULL DEFAULT 0,
  PRIMARY KEY (form_id, device)
);
