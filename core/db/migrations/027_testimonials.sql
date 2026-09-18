CREATE TABLE IF NOT EXISTS testimonials (
    id           BIGSERIAL PRIMARY KEY,
    author_name  TEXT NOT NULL,
    author_title TEXT NOT NULL DEFAULT '',
    body         TEXT NOT NULL,
    is_published BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
