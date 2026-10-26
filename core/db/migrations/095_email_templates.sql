-- Admin overrides for transactional email. No row for a kind means the
-- built-in template in the repo is used. Versions keep the last 20 saves
-- so Reset and Restore do not need a deploy. Schema only: no content rows.
CREATE TABLE IF NOT EXISTS email_templates (
    kind TEXT PRIMARY KEY,
    subject TEXT NOT NULL,
    html TEXT NOT NULL,
    text_body TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS email_template_versions (
    id BIGSERIAL PRIMARY KEY,
    kind TEXT NOT NULL,
    subject TEXT,
    html TEXT,
    text_body TEXT,
    saved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    saved_by TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS email_template_versions_kind_idx ON email_template_versions (kind, saved_at DESC);

DO $$
BEGIN
    IF to_regclass('public.email_templates') IS NOT NULL THEN
        ALTER TABLE public.email_templates ENABLE ROW LEVEL SECURITY;
    END IF;
    IF to_regclass('public.email_template_versions') IS NOT NULL THEN
        ALTER TABLE public.email_template_versions ENABLE ROW LEVEL SECURITY;
    END IF;
END $$;
