-- Paid courses: an optional pay-what-you-want (with a floor) unlock, plus
-- a purchases ledger and per-course gated content.

ALTER TABLE courses ADD COLUMN IF NOT EXISTS is_paid BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE courses ADD COLUMN IF NOT EXISTS min_price_cents INT NOT NULL DEFAULT 0;
ALTER TABLE courses ADD COLUMN IF NOT EXISTS content_key TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS purchases (
    id                SERIAL PRIMARY KEY,
    course_id         INT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    email             TEXT NOT NULL,
    amount_cents      INT NOT NULL,
    currency          TEXT NOT NULL DEFAULT 'USD',
    status            TEXT NOT NULL DEFAULT 'created', -- created | paid | failed
    razorpay_order_id TEXT NOT NULL UNIQUE,
    razorpay_payment_id TEXT NOT NULL DEFAULT '',
    access_token      TEXT NOT NULL UNIQUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at           TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_purchases_course ON purchases (course_id, status);
CREATE INDEX IF NOT EXISTS idx_purchases_access_token ON purchases (access_token);

ALTER TABLE purchases ENABLE ROW LEVEL SECURITY;
