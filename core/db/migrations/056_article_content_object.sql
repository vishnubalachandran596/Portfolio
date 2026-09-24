-- Article HTML uploaded from the admin console lives in the same private
-- bucket as paid lessons (course-content), at articles/{slug}.html.
-- content_object is that path. An empty path means the post is still the
-- articles.body column. This file does not copy existing bodies and does
-- not upload anything — the bucket and its reader policy already exist
-- from migration 055, and that policy covers every object in the bucket.

ALTER TABLE articles ADD COLUMN IF NOT EXISTS content_object TEXT NOT NULL DEFAULT '';
