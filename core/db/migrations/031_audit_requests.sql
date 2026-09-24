-- Fixed-price "Architecture Review" / technical due diligence engagement:
-- a scoped one-time package, not billed hourly. Modeled like bookings
-- (pending_payment -> paid) rather than courses (no pay-what-you-want).
CREATE TABLE IF NOT EXISTS audit_requests (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT NOT NULL,
    email               TEXT NOT NULL,
    company             TEXT NOT NULL DEFAULT '',
    brief               TEXT NOT NULL DEFAULT '',
    amount_cents        INTEGER NOT NULL,
    currency            TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending_payment',
    razorpay_order_id   TEXT NOT NULL UNIQUE,
    razorpay_payment_id TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at             TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_audit_requests_status ON audit_requests (status);
