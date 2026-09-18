-- Lets an article be original, in-house writing instead of only a curated
-- link out to someone else's blog: when body is non-empty, the article's
-- own page renders it in full instead of pointing at source_url.
ALTER TABLE articles ADD COLUMN IF NOT EXISTS body TEXT NOT NULL DEFAULT '';
