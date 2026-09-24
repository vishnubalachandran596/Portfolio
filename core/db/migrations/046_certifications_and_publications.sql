-- Two new About-page sections: Certifications, and Speaking & Publications.
--
-- Deliberately created EMPTY. Both are real content only Vishnu can supply,
-- and a fabricated certification or talk is worse than an absent one — so
-- there are no seeded rows here, not even an example. The public About page
-- renders nothing at all for a section with no rows (no heading, no empty
-- shell, no "coming soon"), so shipping them empty is invisible until he
-- fills them in at /admin/certifications and /admin/publications.
--
-- Idempotent (rule 4): CREATE TABLE IF NOT EXISTS is natively guarded, and
-- there is no seed INSERT — content is added through the admin console, not
-- a migration. skill_groups historically had no unique constraint on its
-- natural key and `id` is SERIAL, so ON CONFLICT DO NOTHING does not stop
-- duplicate rows on a re-run. Having no rows to insert sidesteps that.

CREATE TABLE IF NOT EXISTS certifications (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    issuer          TEXT NOT NULL DEFAULT '',
    -- Nullable: some certifications carry no date, and a zero date would
    -- render as a real-looking one.
    issued_on       DATE,
    -- Nullable rather than defaulted: null means "does not expire", which
    -- is different from "expires and we don't know when".
    expires_on      DATE,
    credential_url  TEXT NOT NULL DEFAULT '',
    is_published    BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_certifications_published ON certifications (is_published, sort_order);

CREATE TABLE IF NOT EXISTS publications (
    id              SERIAL PRIMARY KEY,
    title           TEXT NOT NULL,
    -- Free text rather than an enum: "Conference talk", "Podcast",
    -- "Guest article", "Paper" and whatever comes next all belong here, and
    -- a CHECK constraint would mean a migration every time a new kind
    -- appears. Rendered as a label beside the title.
    kind            TEXT NOT NULL DEFAULT '',
    venue           TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL DEFAULT '',
    published_on    DATE,
    summary         TEXT NOT NULL DEFAULT '',
    is_published    BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_publications_published ON publications (is_published, sort_order);
