-- Tags for the course catalog, same shape as articles.tags (migration
-- 040-era): a real text[] column, not a comma-joined string, so
-- `tag = ANY(tags)` filtering and `unnest()`-based tag counts work the
-- same way CourseRepo.ListAllTags mirrors ArticleRepo.ListAllTags.
ALTER TABLE courses ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';
