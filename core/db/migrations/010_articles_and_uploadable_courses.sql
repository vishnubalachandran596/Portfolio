-- Curated articles: admin adds one by pasting a source link + a short
-- summary (no scraping infra needed — the summary is written once at
-- add-time), shown on /articles and teased on the home page.
CREATE TABLE IF NOT EXISTS articles (
    id            SERIAL PRIMARY KEY,
    title         TEXT NOT NULL,
    summary       TEXT NOT NULL DEFAULT '',
    source_url    TEXT NOT NULL,
    source_name   TEXT NOT NULL DEFAULT '',
    published_date DATE,
    is_published  BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order    INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_articles_published ON articles (is_published, sort_order);
ALTER TABLE articles ENABLE ROW LEVEL SECURITY;

-- Lets the admin upload a course's full HTML content directly from the
-- admin portal (no code deploy needed for a new course): when set, this
-- takes priority over a compiled-in content_key from core/content.
ALTER TABLE courses ADD COLUMN IF NOT EXISTS content_html TEXT NOT NULL DEFAULT '';
