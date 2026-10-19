-- SEO lives in the database, not in templates.
--
-- page_seo holds per-path overrides for the static pages (/, /about,
-- /courses, ...). Every column except path is optional: an empty value
-- means "use the built-in fallback", so a page with no row (or an empty
-- row) renders exactly as it did before this table existed.
--
-- courses / articles / software each get four optional columns. Empty
-- means the fallback chain applies (seo_title -> the existing title
-- pattern; meta_description -> summary -> truncated description;
-- og_image -> the site default), so NEW content inherits sensible values
-- with no code change.
--
-- articles.updated_at is NULL until an article is edited after this
-- migration; the sitemap falls back to published_date/created_at, so
-- lastmod only moves on a real edit.
--
-- Fully idempotent: this file re-runs on every cold start.
CREATE TABLE IF NOT EXISTS page_seo (
    path TEXT PRIMARY KEY,
    seo_title TEXT NOT NULL DEFAULT '',
    meta_description TEXT NOT NULL DEFAULT '',
    primary_keyword TEXT NOT NULL DEFAULT '',
    og_image TEXT NOT NULL DEFAULT '',
    noindex BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE page_seo ENABLE ROW LEVEL SECURITY;

ALTER TABLE courses ADD COLUMN IF NOT EXISTS seo_title TEXT NOT NULL DEFAULT '';
ALTER TABLE courses ADD COLUMN IF NOT EXISTS meta_description TEXT NOT NULL DEFAULT '';
ALTER TABLE courses ADD COLUMN IF NOT EXISTS primary_keyword TEXT NOT NULL DEFAULT '';
ALTER TABLE courses ADD COLUMN IF NOT EXISTS og_image TEXT NOT NULL DEFAULT '';

ALTER TABLE articles ADD COLUMN IF NOT EXISTS seo_title TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS meta_description TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS primary_keyword TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS og_image TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;

ALTER TABLE software ADD COLUMN IF NOT EXISTS seo_title TEXT NOT NULL DEFAULT '';
ALTER TABLE software ADD COLUMN IF NOT EXISTS meta_description TEXT NOT NULL DEFAULT '';
ALTER TABLE software ADD COLUMN IF NOT EXISTS primary_keyword TEXT NOT NULL DEFAULT '';
ALTER TABLE software ADD COLUMN IF NOT EXISTS og_image TEXT NOT NULL DEFAULT '';
