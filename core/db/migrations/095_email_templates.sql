-- Admin overrides for transactional email. No row for a kind means the
-- built-in template in the repo is used. Versions keep the last 20 saves
-- so Reset and Restore do not need a deploy. Schema only: no content rows.
--
-- CREATE TABLE IF NOT EXISTS only looks in pg_class. The table's row type
-- is a separate pg_type row, and that insert is what raises
-- pg_type_typname_nsp_index (23505) when two cold starts both pass the
-- existence check, or when a composite type that is not a table already
-- has the name. relkind 'r'/'p' is a real table and is never dropped.
-- relkind 'c' is a standalone composite type; only that may be dropped.

DO $$
DECLARE
    rel_kind "char";
BEGIN
    SELECT c.relkind INTO rel_kind
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relname = 'email_templates';

    IF rel_kind IN ('r', 'p') THEN
        RETURN;
    END IF;
    IF rel_kind = 'c' THEN
        DROP TYPE public.email_templates;
    ELSIF rel_kind IS NOT NULL THEN
        RAISE EXCEPTION 'public.email_templates is relkind %; refusing to replace it', rel_kind;
    END IF;

    BEGIN
        CREATE TABLE IF NOT EXISTS public.email_templates (
            kind TEXT PRIMARY KEY,
            subject TEXT NOT NULL,
            html TEXT NOT NULL,
            text_body TEXT NOT NULL DEFAULT '',
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_by TEXT NOT NULL DEFAULT ''
        );
    EXCEPTION
        WHEN unique_violation OR duplicate_table OR duplicate_object THEN
            IF to_regclass('public.email_templates') IS NULL THEN
                RAISE;
            END IF;
    END;
END $$;

DO $$
DECLARE
    rel_kind "char";
BEGIN
    SELECT c.relkind INTO rel_kind
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relname = 'email_template_versions';

    IF rel_kind IN ('r', 'p') THEN
        RETURN;
    END IF;
    IF rel_kind = 'c' THEN
        DROP TYPE public.email_template_versions;
    ELSIF rel_kind IS NOT NULL THEN
        RAISE EXCEPTION 'public.email_template_versions is relkind %; refusing to replace it', rel_kind;
    END IF;

    BEGIN
        CREATE TABLE IF NOT EXISTS public.email_template_versions (
            id BIGSERIAL PRIMARY KEY,
            kind TEXT NOT NULL,
            subject TEXT,
            html TEXT,
            text_body TEXT,
            saved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            saved_by TEXT NOT NULL DEFAULT ''
        );
    EXCEPTION
        WHEN unique_violation OR duplicate_table OR duplicate_object THEN
            IF to_regclass('public.email_template_versions') IS NULL THEN
                RAISE;
            END IF;
    END;
END $$;

DO $$
BEGIN
    CREATE INDEX IF NOT EXISTS email_template_versions_kind_idx
        ON public.email_template_versions (kind, saved_at DESC);
EXCEPTION
    WHEN unique_violation OR duplicate_table OR duplicate_object THEN
        IF to_regclass('public.email_template_versions_kind_idx') IS NULL THEN
            RAISE;
        END IF;
END $$;

DO $$
BEGIN
    IF to_regclass('public.email_templates') IS NOT NULL THEN
        ALTER TABLE public.email_templates ENABLE ROW LEVEL SECURITY;
    END IF;
    IF to_regclass('public.email_template_versions') IS NOT NULL THEN
        ALTER TABLE public.email_template_versions ENABLE ROW LEVEL SECURITY;
    END IF;
END $$;
