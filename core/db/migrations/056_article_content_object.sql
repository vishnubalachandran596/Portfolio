-- Article HTML uploaded from the admin console is recorded on
-- articles.content_object (articles/{slug}.html). The bytes live in the
-- private `articles` bucket, created in migration 057, not in this column
-- and not in the course-content bucket. An empty path means the post is
-- still the articles.body column. This file does not copy existing bodies
-- and does not upload anything.

ALTER TABLE articles ADD COLUMN IF NOT EXISTS content_object TEXT NOT NULL DEFAULT '';
