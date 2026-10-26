-- Transactional email delivery metadata. The rendered body is never stored;
-- a retry rebuilds the message from the purchase, booking, or subscriber row.
-- (kind, entity_id) is unique so a second payment verify cannot double-send.
CREATE TABLE IF NOT EXISTS email_outbox (
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

CREATE UNIQUE INDEX IF NOT EXISTS email_outbox_kind_entity_uidx ON email_outbox (kind, entity_id);
CREATE INDEX IF NOT EXISTS email_outbox_retry_idx ON email_outbox (status, updated_at);

-- Same RLS lockdown as migration 076: ENABLE ROW LEVEL SECURITY with no
-- policies denies PostgREST. The app connects as the table owner and bypasses
-- RLS. Enabling it on an already-enabled table is a no-op.
DO $$
BEGIN
    IF to_regclass('public.email_outbox') IS NOT NULL THEN
        ALTER TABLE public.email_outbox ENABLE ROW LEVEL SECURITY;
    END IF;
END $$;
