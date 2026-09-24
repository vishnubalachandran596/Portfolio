-- bookings.status, purchases.status and audit_requests.status are all plain
-- TEXT with no CHECK, and the app reads them with string comparisons. A typo
-- in a hand-written UPDATE, or a column default nobody revisited, produces a
-- row that every code path silently treats as "not paid" / "not confirmed".
--
-- bookings.status DEFAULT is 'pending' (001_schema.sql), but the app only
-- knows 'pending_payment', 'confirmed' and 'cancelled'. A leftover 'pending'
-- row is invisible to ListUpcoming and does not block its own slot.
--
-- CLEANUP FIRST: the CHECK below would FAIL against that existing row. The
-- UPDATE has to run before the ADD CONSTRAINT, in this file, in this order.
--
-- Also: purchases_user_id_fkey historically had no ON DELETE action, unlike
-- every sibling FK to users. SET NULL matches bookings/testimonials/etc. so
-- a purchase survives account deletion with the buyer anonymised.
--
-- Idempotency (rule 4): the UPDATE is scoped to the bad value so it is a
-- no-op on a second pass, ALTER COLUMN SET DEFAULT is naturally idempotent,
-- and every ADD CONSTRAINT is wrapped in the pg_constraint guard.

UPDATE bookings SET status = 'cancelled' WHERE status = 'pending';

ALTER TABLE bookings ALTER COLUMN status SET DEFAULT 'pending_payment';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'bookings_status_check') THEN
        ALTER TABLE bookings ADD CONSTRAINT bookings_status_check
            CHECK (status IN ('pending_payment', 'confirmed', 'cancelled'));
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'purchases_status_check') THEN
        ALTER TABLE purchases ADD CONSTRAINT purchases_status_check
            CHECK (status IN ('created', 'paid', 'failed'));
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'audit_requests_status_check') THEN
        ALTER TABLE audit_requests ADD CONSTRAINT audit_requests_status_check
            CHECK (status IN ('pending_payment', 'paid'));
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'purchases_paid_at_check') THEN
        ALTER TABLE purchases ADD CONSTRAINT purchases_paid_at_check
            CHECK (status <> 'paid' OR paid_at IS NOT NULL);
    END IF;

    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'purchases_user_id_fkey' AND confdeltype = 'a'
    ) THEN
        ALTER TABLE purchases DROP CONSTRAINT purchases_user_id_fkey;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'purchases_user_id_fkey') THEN
        ALTER TABLE purchases ADD CONSTRAINT purchases_user_id_fkey
            FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
    END IF;
END $$;
