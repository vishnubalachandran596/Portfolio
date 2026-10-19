-- Guest checkout: a course or software purchase can be made with an email
-- only (user_id NULL). The site sends no email, so the buyer is shown an
-- unguessable claim code on the success page and later attaches the
-- purchase to an account at /account/claim (or by signing in with Google
-- using the same, Google-verified email). A typed or password-signup email
-- is never trusted to claim anything.
--   claim_code  stored normalised (upper-case hex, no hyphens); '' once the
--               purchase belongs to an account (single use) or never needed
--               one (a signed-in buyer).
--   claimed_at  when the purchase was attached to an account.
-- Fully idempotent: this file re-runs on every cold start.
ALTER TABLE purchases ADD COLUMN IF NOT EXISTS claim_code TEXT NOT NULL DEFAULT '';
ALTER TABLE purchases ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS idx_purchases_claim_code ON purchases (claim_code) WHERE claim_code <> '';
CREATE INDEX IF NOT EXISTS idx_purchases_guest_email ON purchases (lower(email)) WHERE user_id IS NULL AND claim_code <> '';

ALTER TABLE software_purchases ADD COLUMN IF NOT EXISTS claim_code TEXT NOT NULL DEFAULT '';
ALTER TABLE software_purchases ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS idx_software_purchases_claim_code ON software_purchases (claim_code) WHERE claim_code <> '';
CREATE INDEX IF NOT EXISTS idx_software_purchases_guest_email ON software_purchases (lower(email)) WHERE user_id IS NULL AND claim_code <> '';

-- Optional voluntary contribution a learner can add after finishing a
-- course (the course price itself is fixed). One row per Razorpay order;
-- the amount is chosen by the learner but stored and charged server-side,
-- and only a signature-verified order is marked paid.
CREATE TABLE IF NOT EXISTS course_contributions (
    id BIGSERIAL PRIMARY KEY,
    course_id BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    email TEXT NOT NULL,
    amount_cents INT NOT NULL CHECK (amount_cents > 0),
    currency TEXT NOT NULL DEFAULT 'INR',
    status TEXT NOT NULL DEFAULT 'created',
    razorpay_order_id TEXT NOT NULL UNIQUE,
    razorpay_payment_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_course_contributions_course ON course_contributions (course_id, created_at DESC);
ALTER TABLE course_contributions ENABLE ROW LEVEL SECURITY;
