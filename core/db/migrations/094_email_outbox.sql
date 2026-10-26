-- Transactional email delivery metadata. The rendered body is never stored;
-- a retry rebuilds the message from the purchase, booking, or subscriber row.
-- (kind, entity_id) is unique so a second payment verify cannot double-send.
--
-- Same pg_type guard as 095: CREATE TABLE IF NOT EXISTS does not notice a
-- composite type that is not a table, and two cold starts can both pass the
-- pg_class check and collide on pg_type_typname_nsp_index (23505). A real
-- table (relkind 'r'/'p') is left untouched, rows included.

DO $$
DECLARE
    rel_kind "char";
BEGIN
    SELECT c.relkind INTO rel_kind
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relname = 'email_outbox';

    IF rel_kind IN ('r', 'p') THEN
        RETURN;
    END IF;
    IF rel_kind = 'c' THEN
        DROP TYPE public.email_outbox;
    ELSIF rel_kind IS NOT NULL THEN
        RAISE EXCEPTION 'public.email_outbox is relkind %; refusing to replace it', rel_kind;
    END IF;

    BEGIN
        CREATE TABLE IF NOT EXISTS public.email_outbox (
            id BIGSERIAL PRIMARY KEY,
            kind TEXT NOT NULL,
            entity_id TEXT NOT NULL,
            to_email TEXT NOT NULL,
            provider TEXT NOT NULL,
            status TEXT NOT NULL DEFAULT 'pending',
            attempts INT NOT NULL DEFAULT 0,
            last_error TEXT NOT NULL DEFAULT '',
            provider_id TEXT NOT NULL DEFAULT '',
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            sent_at TIMESTAMPTZ
        );
    EXCEPTION
        WHEN unique_violation OR duplicate_table OR duplicate_object THEN
            IF to_regclass('public.email_outbox') IS NULL THEN
                RAISE;
            END IF;
    END;
END $$;

DO $$
BEGIN
    CREATE UNIQUE INDEX IF NOT EXISTS email_outbox_kind_entity_uidx
        ON public.email_outbox (kind, entity_id);
EXCEPTION
    WHEN unique_violation OR duplicate_table OR duplicate_object THEN
        IF to_regclass('public.email_outbox_kind_entity_uidx') IS NULL THEN
            RAISE;
        END IF;
END $$;

DO $$
BEGIN
    CREATE INDEX IF NOT EXISTS email_outbox_retry_idx
        ON public.email_outbox (status, updated_at);
EXCEPTION
    WHEN unique_violation OR duplicate_table OR duplicate_object THEN
        IF to_regclass('public.email_outbox_retry_idx') IS NULL THEN
            RAISE;
        END IF;
END $$;

-- Same RLS lockdown as migration 076: ENABLE ROW LEVEL SECURITY with no
-- policies denies PostgREST. The app connects as the table owner and bypasses
-- RLS. Enabling it on an already-enabled table is a no-op.
DO $$
BEGIN
    IF to_regclass('public.email_outbox') IS NOT NULL THEN
        ALTER TABLE public.email_outbox ENABLE ROW LEVEL SECURITY;
    END IF;
END $$;
