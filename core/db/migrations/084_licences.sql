-- Temple Accounts (and any future licensed software) licence keys.
-- A licence row is created for every paid software purchase (one per
-- purchase, never two) or by an admin (purchase_id NULL). The customer
-- later supplies a Device ID on /account/licences and the server signs a
-- key with the Ed25519 private key held in the LICENCE_SIGNING_KEY_B64
-- environment secret. Nothing secret is stored here: only the signed key
-- (public by nature) and an audit trail.
-- Fully idempotent: this file re-runs on every cold start.
ALTER TABLE software ADD COLUMN IF NOT EXISTS licence_product TEXT NOT NULL DEFAULT '';
ALTER TABLE software ADD COLUMN IF NOT EXISTS licence_versions TEXT NOT NULL DEFAULT '1';

CREATE TABLE IF NOT EXISTS licences (
    id BIGSERIAL PRIMARY KEY,
    lic_code TEXT NOT NULL UNIQUE,
    software_id BIGINT NOT NULL REFERENCES software(id) ON DELETE CASCADE,
    purchase_id BIGINT REFERENCES software_purchases(id) ON DELETE SET NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ver TEXT NOT NULL DEFAULT '',
    device_hash TEXT NOT NULL DEFAULT '',
    key_text TEXT NOT NULL DEFAULT '',
    kid TEXT NOT NULL DEFAULT '',
    iat TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    revoked_reason TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    added_by_admin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One licence per purchase: makes creation safe to retry.
CREATE UNIQUE INDEX IF NOT EXISTS idx_licences_purchase ON licences(purchase_id) WHERE purchase_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_licences_user ON licences(user_id);

CREATE TABLE IF NOT EXISTS licence_events (
    id BIGSERIAL PRIMARY KEY,
    licence_id BIGINT NOT NULL REFERENCES licences(id) ON DELETE CASCADE,
    event TEXT NOT NULL,
    actor TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_licence_events_lic ON licence_events(licence_id, event, created_at);

ALTER TABLE licences ENABLE ROW LEVEL SECURITY;
ALTER TABLE licence_events ENABLE ROW LEVEL SECURITY;
