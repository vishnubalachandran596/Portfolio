-- Case Studies: short Problem -> Approach -> Result write-ups shown on both
-- About and Consultancy as concrete proof of work, alongside (not
-- replacing) Testimonials. Same reasoning as migration 046's Certifications
-- and Speaking & Publications: this is real engagement detail only Vishnu
-- can supply accurately, and the Consultancy FAQ's confidentiality answer
-- already promises nothing about a client appears anywhere on this site
-- without their explicit agreement -- so this ships EMPTY, no seeded
-- example row, same as 046. The public pages render nothing at all for
-- this section until it has at least one published row.
CREATE TABLE IF NOT EXISTS case_studies (
    id             BIGSERIAL PRIMARY KEY,
    title          TEXT NOT NULL,
    client_context TEXT NOT NULL DEFAULT '',
    problem        TEXT NOT NULL DEFAULT '',
    approach       TEXT NOT NULL DEFAULT '',
    result         TEXT NOT NULL DEFAULT '',
    is_published   BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order     INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- RLS lockdown, same as migration 051 did for every table added since 003 --
-- this table is new, so it needs the same guard from day one rather than a
-- follow-up migration.
ALTER TABLE case_studies ENABLE ROW LEVEL SECURITY;

-- Natural-key uniqueness on `title`, so the bootstrap-data snapshot can
-- upsert this table the same way it upserts skill_groups/experiences/etc.
-- (see migration 054's version of this note for certifications).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'case_studies_title_uk') THEN
        ALTER TABLE case_studies ADD CONSTRAINT case_studies_title_uk UNIQUE (title);
    END IF;
END $$;
