-- One licence key covers every app version and every build (Microsoft
-- Store, offline Windows zip, Mac, Linux) on the same computer, so the
-- signed payload no longer carries "ver" and nothing ties a licence to a
-- version any more.
--   * software.licence_versions and licences.ver are dropped (nothing in
--     the code reads them after this change; existing licence rows, keys,
--     devices and events are untouched).
--   * software.microsoft_store_url: nullable, edited in the admin software
--     form. NULL/empty makes the downloads block say "Coming soon".
-- Every statement is idempotent (IF EXISTS / IF NOT EXISTS) because this
-- file re-runs on a cold start and a migration error is fatal to the app.
ALTER TABLE software ADD COLUMN IF NOT EXISTS microsoft_store_url TEXT;
ALTER TABLE software DROP COLUMN IF EXISTS licence_versions;
ALTER TABLE licences DROP COLUMN IF EXISTS ver;
