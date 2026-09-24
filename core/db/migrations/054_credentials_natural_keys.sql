-- Natural-key uniqueness for the two tables migration 046 added, so a
-- snapshot can upsert them the way it upserts every other content table.
-- 046 shipped no seed rows, so there is nothing to clean up first.
--
-- NULLS NOT DISTINCT is the load-bearing part. issued_on and published_on
-- are nullable by design ("this certification carries no date"), and under
-- default UNIQUE semantics two NULLs never conflict — so a plain
-- UNIQUE (name, issuer, issued_on) would let the same undated certification
-- insert on every pass. Requires Postgres 15+; the project is on 17.
--
-- The key includes the date so that a renewal (same name and issuer, later
-- issued_on) stays a separate row.
--
-- Idempotent (rule 4): ADD CONSTRAINT has no IF NOT EXISTS form, so both are
-- wrapped in the pg_constraint guard.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'certifications_natural_uk') THEN
        ALTER TABLE certifications ADD CONSTRAINT certifications_natural_uk
            UNIQUE NULLS NOT DISTINCT (name, issuer, issued_on);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'publications_natural_uk') THEN
        ALTER TABLE publications ADD CONSTRAINT publications_natural_uk
            UNIQUE NULLS NOT DISTINCT (title, venue, published_on);
    END IF;
END $$;
