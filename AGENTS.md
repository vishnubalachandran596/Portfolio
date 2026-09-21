# Agent instructions for this repo — see also CLAUDE.md and .cursor/rules/architecture.mdc (full detail)

This file exists for Codex and any other AGENTS.md-reading tool. Claude Code reads `CLAUDE.md`; Cursor reads `.cursor/rules/architecture.mdc` and `.cursorrules`. All four files carry the same rules — this one is the condensed version. If you're an agent working on this repo, read `docs/FUNCTIONALITY.md` and `docs/ARCHITECTURE.md` first, and `CLAUDE.md` for the full rationale behind everything below.

Before any change: read `docs/FUNCTIONALITY.md` (what the product does) and `docs/ARCHITECTURE.md` (how it's built). Update whichever doc your change affects, in the same commit — not a follow-up.

## Core rules

1. **Reuse existing utilities** (e.g. `models.FormatMoney`, `slugify()`, `resolvePromoCode()`, `authutil.RandomHex()`) — search before writing a near-duplicate. A duplicate money-formatting function once silently disagreed with the real one, and duplicated random-token generation existed in four files; both were consolidated. That's the bug class this rule prevents.
2. **New features follow the layered pattern additively:** `models` -> `repo` (all SQL) -> `handlers` -> `templates` -> `routes.go`. Reference: `core/repo/audit_requests.go` + `consultancy.go`'s Audit checkout — a 4th paid product added without touching existing checkout code.
3. **Design for horizontal scale:** stateless serverless function, no in-process state across requests except the pooled DB connection. Run independent DB lookups on hot paths concurrently.
4. **Every migration must be idempotent — this caused a real production outage.** `Migrate()` re-runs every un-recorded `.sql` file on every cold start, and a migration error is fatal for the whole app. `CREATE TABLE IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` are safe as-is. `ADD CONSTRAINT` has no `IF NOT EXISTS` form in Postgres — wrap it in `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '...') THEN ALTER TABLE ... ADD CONSTRAINT ...; END IF; END $$;`. Applying it manually to unblock work does not make the checked-in file safe — it still runs for real on the next cold start (this is exactly what took the site down once — migration 034, hotfixed in `7601c8a`).
5. **Never commit secrets.** `.env`/`.env.local` stay gitignored; `.env.example` holds placeholders only. Check the diff before pushing anything config-adjacent.
6. **Payment/access confirmation is server-side (HMAC-verified) only** — never trust a client-reported success.
7. **Stay consistent with the stack:** Go stdlib `net/http`, server-rendered `html/template`, plain CSS (existing design tokens), raw SQL via `database/sql`, rem units. No framework additions for a single feature.

## Before finishing any task in this repo

- Did this change make `docs/FUNCTIONALITY.md` or `docs/ARCHITECTURE.md` stale? Update it now.
- Did I reuse an existing helper/pattern instead of writing a near-duplicate?
- Does a new feature follow the models -> repo -> handlers -> templates -> routes layering as its own additive piece?
- Any new/changed `.sql` migration — is it safe to run twice? (See rule 4.)
- Any real secret in the diff? Check, don't assume.
- `go build ./... && go vet ./...` clean? Any changed template smoke-tested?

Full detail and rationale live in `CLAUDE.md` and `.cursor/rules/architecture.mdc` — read those for the complete picture.
