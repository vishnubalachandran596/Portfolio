-- Thumbs-up is now restricted to logged-in visitors (see handlers.ArticleThumbsUp),
-- so this constraint is the authoritative "has this user already voted"
-- check going forward — enforced at the database, not just a cookie, so it
-- holds across browsers/devices for the same account and survives a race
-- between two concurrent requests.
--
-- Wrapped in a guard because "ADD CONSTRAINT" has no "IF NOT EXISTS" form
-- in Postgres — unlike every other migration in this directory, which is
-- naturally idempotent (CREATE TABLE IF NOT EXISTS, ADD COLUMN IF NOT
-- EXISTS). Without the guard, this migration fails every cold start once
-- applied once, which is fatal: Bootstrap() treats a migration error as
-- fatal for the whole app, not just this feature.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'article_thumbs_up_article_user_uk'
    ) THEN
        ALTER TABLE article_thumbs_up ADD CONSTRAINT article_thumbs_up_article_user_uk UNIQUE (article_id, user_id);
    END IF;
END $$;
