-- Ties a course purchase to the buyer's account when they were logged in
-- at checkout, so /account can list "my courses" reliably by user_id
-- rather than just matching on email. Guest checkouts (not logged in)
-- keep user_id NULL — email remains the only identifier for those.
ALTER TABLE purchases ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_purchases_user_id ON purchases (user_id);
CREATE INDEX IF NOT EXISTS idx_bookings_user_id ON bookings (user_id);
