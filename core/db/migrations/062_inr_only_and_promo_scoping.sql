-- Two unrelated changes bundled because both ship in the same commit:
--
-- 1. INR-only pricing. Multi-currency (USD international / INR for India,
--    detected from the Vercel edge's X-Vercel-IP-Country header) is
--    retired -- every course now sells in INR only. courses.min_price_cents
--    already holds the real industry-level price for most rows (the Oct
--    2026 pricing pass set it in USD cents); this migration folds the
--    INR figure that already existed in min_price_cents_inr into
--    min_price_cents (the column the app now reads exclusively), then
--    drops the two columns the dual-currency code no longer touches
--    (min_price_cents_inr, currency). The UPDATE is naturally idempotent:
--    re-running it after the INR column is gone is a no-op because the
--    WHEN clause's source column won't exist to read from the second
--    time -- see the guarded DO block below.
--
-- 2. Promo code scoping upgrade: a configurable per-user redemption limit
--    (max_redemptions_per_user, 0 = unlimited -- replaces the old
--    hard-coded "once ever per code" behavior with "once ever per code
--    per account" now configurable to N), a start time to pair with the
--    existing expires_at, a public/private flag (public codes scroll in
--    the sitewide promo banner; private ones never do), and an
--    allowed_emails column for user-scoped private codes (comma-separated
--    text -- deliberately not a join table, since this is a short,
--    rarely-edited list entered as one textarea in the admin form).
--    applies_to_all_courses is dropped: "no courses selected" now means
--    "applies to any course" by itself, so the column was redundant with
--    promo_code_courses being empty.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'courses' AND column_name = 'min_price_cents_inr') THEN
        UPDATE courses SET min_price_cents = min_price_cents_inr WHERE min_price_cents_inr > 0;
        UPDATE courses SET price_cents = min_price_cents WHERE is_paid;
    END IF;
END $$;

ALTER TABLE courses DROP COLUMN IF EXISTS min_price_cents_inr;
ALTER TABLE courses DROP COLUMN IF EXISTS currency;

ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS max_redemptions_per_user INT NOT NULL DEFAULT 1;
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS starts_at TIMESTAMPTZ NOT NULL DEFAULT '2000-01-01T00:00:00Z';
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS is_public BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS allowed_emails TEXT NOT NULL DEFAULT '';
ALTER TABLE promo_codes DROP COLUMN IF EXISTS applies_to_all_courses;
