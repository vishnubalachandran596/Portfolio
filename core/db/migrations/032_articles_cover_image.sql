-- A blog post's featured/cover image — shown at the top of the post and
-- as a thumbnail in the articles list, separate from any images embedded
-- directly in the HTML body.
ALTER TABLE articles ADD COLUMN IF NOT EXISTS cover_image_url TEXT NOT NULL DEFAULT '';
