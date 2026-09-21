-- Thumbs-up is now restricted to logged-in visitors (see handlers.ArticleThumbsUp),
-- so this constraint is the authoritative "has this user already voted"
-- check going forward — enforced at the database, not just a cookie, so it
-- holds across browsers/devices for the same account and survives a race
-- between two concurrent requests.
ALTER TABLE article_thumbs_up ADD CONSTRAINT article_thumbs_up_article_user_uk UNIQUE (article_id, user_id);
