-- Removes the external/curated-article feature outright. An article is now
-- original content with a body, full stop — there is no longer any way to
-- store (or enter, see core/handlers/articles.go) a row that is merely a
-- link to something hosted elsewhere.
--
-- The DELETE is feature removal, not content authoring: it drops leftover
-- link-only rows (non-empty source_url and empty body) so a database that
-- still has the old columns cannot keep curated link-outs after this file
-- runs. Production already holds only original posts, so the DELETE is a
-- no-op there. It is deliberately not keyed off ids, which differ between
-- environments.
--
-- Idempotency (rule 4): the DELETE has to be guarded, not just the DDL.
-- `DELETE FROM articles WHERE source_url <> ''` would fail outright on a
-- second pass with "column source_url does not exist", since the ALTERs
-- below have removed it by then — a migration that errors on re-run is
-- exactly the failure mode that took the site down once. So the DELETE runs
-- inside a DO block, gated on the column still existing, and via EXECUTE so
-- the statement is never even parsed when the branch isn't taken.
-- DROP COLUMN IF EXISTS is natively idempotent and needs no guard.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'articles'
          AND column_name = 'source_url'
    ) THEN
        EXECUTE $q$DELETE FROM articles WHERE source_url <> '' AND body = ''$q$;
    END IF;
END $$;

ALTER TABLE articles DROP COLUMN IF EXISTS source_url;
ALTER TABLE articles DROP COLUMN IF EXISTS source_name;
