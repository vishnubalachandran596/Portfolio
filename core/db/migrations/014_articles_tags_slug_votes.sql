-- Gives each article its own internal page (so it can appear in the
-- sitemap and actually be indexed — an outbound-only card never was),
-- plus tags for filtering and a thumbs-up counter for sorting by what
-- readers found useful.
ALTER TABLE articles ADD COLUMN IF NOT EXISTS slug TEXT;
ALTER TABLE articles ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS thumbs_up_count INT NOT NULL DEFAULT 0;

ALTER TABLE articles ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_articles_slug ON articles (slug);
CREATE INDEX IF NOT EXISTS idx_articles_tags ON articles USING GIN (tags);
