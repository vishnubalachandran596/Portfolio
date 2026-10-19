-- 086: permanent redirects for renamed course URLs, and a flag for courses
-- whose lesson has a public free preview (see storage.LessonPreview).
-- slug_redirects is read only on a would-be 404 (App.NotFound), so a hit
-- costs nothing on any live route. Fully idempotent: re-runs every cold start.
CREATE TABLE IF NOT EXISTS slug_redirects (
    old_path   TEXT PRIMARY KEY,
    new_path   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE slug_redirects ENABLE ROW LEVEL SECURITY;

ALTER TABLE courses ADD COLUMN IF NOT EXISTS preview_ready BOOLEAN NOT NULL DEFAULT FALSE;
