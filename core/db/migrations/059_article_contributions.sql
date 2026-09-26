-- Optional pay-what-you-want support on every article. Same visitor may
-- contribute more than once, so there is no UNIQUE(article_id, user_id).
-- Schema only — no content rows.
CREATE TABLE IF NOT EXISTS article_contributions (
    id                  BIGSERIAL PRIMARY KEY,
    article_id          BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    user_id             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    email               TEXT NOT NULL,
    amount_cents        INTEGER NOT NULL,
    currency            TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'created',
    razorpay_order_id   TEXT NOT NULL UNIQUE,
    razorpay_payment_id TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at             TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_article_contributions_article
    ON article_contributions (article_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_article_contributions_user
    ON article_contributions (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_article_contributions_status
    ON article_contributions (status);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'article_contributions_status_check') THEN
        ALTER TABLE article_contributions ADD CONSTRAINT article_contributions_status_check
            CHECK (status IN ('created', 'paid', 'failed'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'article_contributions_paid_at_check') THEN
        ALTER TABLE article_contributions ADD CONSTRAINT article_contributions_paid_at_check
            CHECK (status <> 'paid' OR paid_at IS NOT NULL);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'article_contributions_amount_check') THEN
        ALTER TABLE article_contributions ADD CONSTRAINT article_contributions_amount_check
            CHECK (amount_cents > 0);
    END IF;
END $$;

ALTER TABLE article_contributions ENABLE ROW LEVEL SECURITY;
