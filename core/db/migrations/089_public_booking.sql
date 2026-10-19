-- Public, no-login booking (W3). The site sends no email, so a booking is
-- identified on screen by a short code (BK-XXXXXX) and managed through a
-- private link whose token is shown once and stored only as a SHA-256 hash.
-- Every statement here is safe to run twice.

ALTER TABLE bookings ADD COLUMN IF NOT EXISTS booking_code       TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS manage_token_hash  TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_bookings_booking_code
    ON bookings (booking_code) WHERE booking_code <> '';
CREATE UNIQUE INDEX IF NOT EXISTS uq_bookings_manage_token
    ON bookings (manage_token_hash) WHERE manage_token_hash <> '';

-- One live booking per slot start. The original UNIQUE (slot_start) also
-- counted cancelled rows, so a cancelled slot could never be booked again.
-- The partial index replaces it before the old constraint is dropped, so the
-- table is never left unprotected.
CREATE UNIQUE INDEX IF NOT EXISTS uq_bookings_active_slot
    ON bookings (slot_start) WHERE status <> 'cancelled';
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_slot_start_key;

-- No two live bookings may overlap in time (covers slots of different
-- lengths, which a plain unique index on the start cannot). Range operators
-- on tstzrange need no extension. If old data already overlaps, skip with a
-- notice rather than failing the whole app: the unique index above still
-- guarantees one booking per slot start.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'bookings_no_overlap') THEN
        BEGIN
            ALTER TABLE bookings ADD CONSTRAINT bookings_no_overlap
                EXCLUDE USING gist (tstzrange(slot_start, slot_end) WITH &&)
                WHERE (status <> 'cancelled');
        EXCEPTION WHEN OTHERS THEN
            RAISE NOTICE 'bookings_no_overlap not added: %', SQLERRM;
        END;
    END IF;
END $$;
