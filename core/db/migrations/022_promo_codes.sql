CREATE TABLE IF NOT EXISTS promo_codes (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    product TEXT NOT NULL CHECK (product IN ('course', 'consultancy')),
    discount_percent INT NOT NULL CHECK (discount_percent > 0 AND discount_percent <= 100),
    applies_to_all_courses BOOLEAN NOT NULL DEFAULT FALSE,
    max_redemptions INT NOT NULL DEFAULT 0,
    redemption_count INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS promo_code_courses (
    promo_code_id BIGINT NOT NULL REFERENCES promo_codes(id) ON DELETE CASCADE,
    course_id BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    PRIMARY KEY (promo_code_id, course_id)
);

ALTER TABLE purchases ADD COLUMN IF NOT EXISTS promo_code_id BIGINT REFERENCES promo_codes(id) ON DELETE SET NULL;
ALTER TABLE purchases ADD COLUMN IF NOT EXISTS promo_code TEXT NOT NULL DEFAULT '';
ALTER TABLE purchases ADD COLUMN IF NOT EXISTS discount_cents INT NOT NULL DEFAULT 0;

ALTER TABLE bookings ADD COLUMN IF NOT EXISTS promo_code_id BIGINT REFERENCES promo_codes(id) ON DELETE SET NULL;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS promo_code TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS discount_cents INT NOT NULL DEFAULT 0;
