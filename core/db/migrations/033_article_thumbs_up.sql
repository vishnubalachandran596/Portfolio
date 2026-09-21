-- Per-vote detail for article thumbs-up, so the admin can see who voted
-- (when the voter was logged in) rather than only the aggregate count
-- already kept on articles.thumbs_up_count. user_id is nullable because
-- voting itself never requires login — an anonymous vote just has no name
-- attached to it in the admin view.
CREATE TABLE IF NOT EXISTS article_thumbs_up (
    id BIGSERIAL PRIMARY KEY,
    article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_article_thumbs_up_article ON article_thumbs_up(article_id);
