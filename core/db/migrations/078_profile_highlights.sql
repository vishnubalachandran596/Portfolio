-- Home-page stat strip ("2M+ / transactions a day"). Content, so it lives
-- here as an empty table edited at /admin/highlights — never seeded from
-- the repo (CLAUDE.md rule 8). Home falls back to profile.years_experience
-- when the table is empty, so a fresh database still renders.
--
-- Idempotent: CREATE TABLE IF NOT EXISTS, and RLS enabled in the same
-- migration as the table (the gap migration 076 had to close for
-- chatbot_reindex_pending). ENABLE ROW LEVEL SECURITY is a no-op on a
-- table that already has it.
CREATE TABLE IF NOT EXISTS profile_highlights (
    id           SERIAL PRIMARY KEY,
    value        TEXT NOT NULL,
    label        TEXT NOT NULL,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    is_published BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE profile_highlights ENABLE ROW LEVEL SECURITY;
