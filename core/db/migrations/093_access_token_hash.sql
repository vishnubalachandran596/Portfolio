-- Course and software access tokens are bearer secrets; store only their
-- SHA-256, like the booking manage token. Idempotent: the backfill only
-- touches rows with no hash yet, and the scrub only rows not yet scrubbed,
-- so a re-run on cold start is a no-op.
ALTER TABLE purchases          ADD COLUMN IF NOT EXISTS access_token_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE software_purchases ADD COLUMN IF NOT EXISTS access_token_hash TEXT NOT NULL DEFAULT '';

UPDATE purchases SET access_token_hash = encode(sha256(convert_to(access_token, 'UTF8')), 'hex')
 WHERE access_token_hash = '' AND access_token NOT LIKE 'scrubbed-%';
UPDATE software_purchases SET access_token_hash = encode(sha256(convert_to(access_token, 'UTF8')), 'hex')
 WHERE access_token_hash = '' AND access_token NOT LIKE 'scrubbed-%';

UPDATE purchases SET access_token = 'scrubbed-' || access_token_hash
 WHERE access_token_hash <> '' AND access_token NOT LIKE 'scrubbed-%';
UPDATE software_purchases SET access_token = 'scrubbed-' || access_token_hash
 WHERE access_token_hash <> '' AND access_token NOT LIKE 'scrubbed-%';

CREATE UNIQUE INDEX IF NOT EXISTS idx_purchases_access_token_hash ON purchases (access_token_hash) WHERE access_token_hash <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_software_purchases_access_token_hash ON software_purchases (access_token_hash) WHERE access_token_hash <> '';
