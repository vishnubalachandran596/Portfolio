-- Redesign: visitor accounts (required to book consultancy or buy a
-- course), and paid consultancy bookings.

-- Visitor accounts, separate from the admins table. An account whose
-- email matches an admin's email is *also* granted an admin session on
-- login (see handlers/auth.go) — one login form, role decided by email.
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- Consultancy bookings now carry payment: a slot is provisionally
-- reserved (status='pending_payment') the moment a Razorpay order is
-- created — the existing UNIQUE(slot_start) constraint stops two buyers
-- racing for the same slot — then flips to 'confirmed' once payment is
-- verified. The availability query treats a pending reservation older
-- than 20 minutes as free again, so an abandoned checkout doesn't need a
-- separate cleanup job.
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS user_id INT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS amount_cents INT NOT NULL DEFAULT 0;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT 'USD';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS razorpay_order_id TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS razorpay_payment_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_bookings_razorpay_order ON bookings (razorpay_order_id) WHERE razorpay_order_id <> '';
