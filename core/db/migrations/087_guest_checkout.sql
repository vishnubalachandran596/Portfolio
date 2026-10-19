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
