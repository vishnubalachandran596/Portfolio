-- Software is the fourth paid product on this site, alongside courses
-- (checkout.go), the consultancy booking flow, and article contributions.
-- Unlike a course's pay-what-you-want pricing, a software item sells at
-- one fixed price. The download itself is never hosted here -- the
-- download_link column holds an external URL (e.g. a Google Drive
-- folder), revealed only once software_purchases confirms payment, the
-- same access-gating shape courseAccess uses for paid course content.
--
-- license_terms and post_purchase_instructions are plain admin-editable
-- text (e.g. "Licensed per device" / "Email support@... after payment for
-- your license key") so a licensing policy change needs no deploy.
CREATE TABLE IF NOT EXISTS software (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    version TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    rag_context TEXT NOT NULL DEFAULT '',
    download_link TEXT NOT NULL DEFAULT '',
    price_cents INT NOT NULL DEFAULT 0,
    license_terms TEXT NOT NULL DEFAULT '',
    post_purchase_instructions TEXT NOT NULL DEFAULT '',
    is_published BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Mirrors purchases (courses) field-for-field, with software_id in place
-- of course_id -- a new parallel table rather than a nullable course_id
-- on purchases, per CLAUDE.md's "new file, not a branch inside an
-- existing one" guidance for a genuinely separate product line.
CREATE TABLE IF NOT EXISTS software_purchases (
    id BIGSERIAL PRIMARY KEY,
    software_id BIGINT NOT NULL REFERENCES software(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    email TEXT NOT NULL,
    amount_cents INT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'INR',
    status TEXT NOT NULL DEFAULT 'created',
    razorpay_order_id TEXT NOT NULL DEFAULT '',
    razorpay_payment_id TEXT NOT NULL DEFAULT '',
    access_token TEXT NOT NULL DEFAULT '',
    promo_code_id BIGINT REFERENCES promo_codes(id) ON DELETE SET NULL,
    promo_code TEXT NOT NULL DEFAULT '',
    promo_text TEXT NOT NULL DEFAULT '',
    discount_cents INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_software_purchases_order ON software_purchases(razorpay_order_id);
CREATE INDEX IF NOT EXISTS idx_software_purchases_token ON software_purchases(access_token);
CREATE INDEX IF NOT EXISTS idx_software_purchases_user ON software_purchases(user_id);

-- Widen the promo_codes.product CHECK to also allow 'software'. A CHECK
-- constraint has no native IF NOT EXISTS/OR REPLACE form, so this drops
-- and recreates it by its fixed, deterministic name -- safe to run any
-- number of times: each run ends with the exact same constraint in place,
-- and DROP ... IF EXISTS never errors even on the very first run before
-- the constraint has this name.
ALTER TABLE promo_codes DROP CONSTRAINT IF EXISTS promo_codes_product_check;
ALTER TABLE promo_codes ADD CONSTRAINT promo_codes_product_check
    CHECK (product IN ('course', 'consultancy', 'software'));

ALTER TABLE software ENABLE ROW LEVEL SECURITY;
ALTER TABLE software_purchases ENABLE ROW LEVEL SECURITY;
