-- Per-item scope for software promo codes, mirroring promo_code_courses.
-- A software-product code with no rows here applies to any software item;
-- with rows, only to those items. Fully idempotent: re-runs on every cold
-- start.
CREATE TABLE IF NOT EXISTS promo_code_software (
    promo_code_id BIGINT NOT NULL REFERENCES promo_codes(id) ON DELETE CASCADE,
    software_id   BIGINT NOT NULL REFERENCES software(id) ON DELETE CASCADE,
    PRIMARY KEY (promo_code_id, software_id)
);
CREATE INDEX IF NOT EXISTS idx_promo_code_software_software ON promo_code_software (software_id);
ALTER TABLE promo_code_software ENABLE ROW LEVEL SECURITY;
