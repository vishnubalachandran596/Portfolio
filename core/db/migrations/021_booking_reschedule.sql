-- Tracks admin-initiated reschedules so the visitor's /account page can
-- show a notice ("your booking moved from X to Y") until they either
-- acknowledge it or pick a different time themselves.
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS previous_slot_start TIMESTAMPTZ;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS rescheduled_at TIMESTAMPTZ;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS reschedule_acknowledged BOOLEAN NOT NULL DEFAULT TRUE;
