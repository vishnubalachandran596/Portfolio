-- Adds a phone number field to consultancy bookings, and a new
-- contact_messages table backing a real submittable form on /contact
-- (which previously was static display cards only, no form). Both
-- statements use IF NOT EXISTS, naturally idempotent — no DO-block
-- guard needed (see CLAUDE.md's migration-idempotency rule).
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS phone text NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS contact_messages (
    id bigserial PRIMARY KEY,
    name text NOT NULL,
    email text NOT NULL,
    phone text NOT NULL DEFAULT '',
    message text NOT NULL,
    is_read boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
