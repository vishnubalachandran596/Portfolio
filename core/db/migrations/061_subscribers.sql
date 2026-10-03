-- Email capture for Courses/Articles/Home: a visitor leaves an email to
-- hear about new courses and posts, nothing more automated than that (see
-- docs/ARCHITECTURE.md -- there is still no transactional email provider
-- wired up anywhere in this codebase, same reason the /contact form sends
-- no confirmation email). Vishnu reaches out manually; this table is just
-- the list.
--
-- Pure visitor PII like contact_messages/bookings, so it is never part of
-- the gen_bootstrap_data.sql content snapshot and gets the same RLS
-- lockdown every table added after migration 003 needs.
CREATE TABLE IF NOT EXISTS subscribers (
    id          BIGSERIAL PRIMARY KEY,
    email       TEXT NOT NULL UNIQUE,
    source_page TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE subscribers ENABLE ROW LEVEL SECURITY;
