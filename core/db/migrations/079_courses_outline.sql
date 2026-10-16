-- "What's inside" on the course detail page: the lesson file's own section
-- titles (its <h2>s, or <h3>s when it has none), one per line. Derived
-- from the stored lesson HTML — written when a lesson is uploaded and
-- backfilled lazily the first time a detail page finds it empty — so it is
-- never typed by hand and never seeded from the repo.
--
-- ADD COLUMN IF NOT EXISTS is naturally idempotent (see CLAUDE.md rule 4).
ALTER TABLE courses ADD COLUMN IF NOT EXISTS outline TEXT NOT NULL DEFAULT '';
