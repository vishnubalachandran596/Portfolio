# Agent instructions for this repo — see also CLAUDE.md and .cursor/rules/architecture.mdc (full detail)

This file exists for Codex and any other AGENTS.md-reading tool. Claude Code reads `CLAUDE.md`; Cursor reads `.cursor/rules/architecture.mdc` and `.cursorrules`. All four files carry the same rules — this one is the condensed version. If you're an agent working on this repo, read `docs/FUNCTIONALITY.md` and `docs/ARCHITECTURE.md` first, and `CLAUDE.md` for the full rationale behind everything below.

Before any change: read `docs/FUNCTIONALITY.md` (what the product does) and `docs/ARCHITECTURE.md` (how it's built). Update whichever doc your change affects, in the same commit — not a follow-up.

## Core rules

1. **Reuse existing utilities** (e.g. `models.FormatMoney`, `slugify()`, `resolvePromoCode()`, `authutil.RandomHex()`) — search before writing a near-duplicate. A duplicate money-formatting function once silently disagreed with the real one, and duplicated random-token generation existed in four files; both were consolidated. That's the bug class this rule prevents.
2. **New features follow the layered pattern additively:** `models` -> `repo` (all SQL) -> `handlers` -> `templates` -> `routes.go`. Reference: `core/repo/audit_requests.go` + `consultancy.go`'s Audit checkout — a 4th paid product added without touching existing checkout code.
3. **Design for horizontal scale:** stateless serverless function, no in-process state across requests except the pooled DB connection. Run independent DB lookups on hot paths concurrently.
4. **Every migration must be idempotent — this caused a real production outage.** `Migrate()` re-runs every un-recorded `.sql` file on every cold start, and a migration error is fatal for the whole app. `CREATE TABLE IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` are safe as-is. `ADD CONSTRAINT` has no `IF NOT EXISTS` form in Postgres — wrap it in `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '...') THEN ALTER TABLE ... ADD CONSTRAINT ...; END IF; END $$;`. Applying it manually to unblock work does not make the checked-in file safe — it still runs for real on the next cold start (this is exactly what took the site down once — migration 034, hotfixed in `7601c8a`).
5. **Never commit secrets.** `.env`/`.env.local` stay gitignored; `.env.example` holds placeholders only. Check the diff before pushing anything config-adjacent. Third-party keys — even ones that end up public in page HTML, like the JDoodle embed keys — live only in Vercel environment variables. Never write their values into code, docs, commit messages, or Mem0.
6. **Payment/access confirmation is server-side (HMAC-verified) only** — never trust a client-reported success.
7. **Stay consistent with the stack:** Go stdlib `net/http`, server-rendered `html/template`, plain CSS (existing design tokens), raw SQL via `database/sql`, rem units. No framework additions for a single feature.

8. **Keep project memory current — write it down as you go.** Agents here share no conversation history, so record in Mem0 (via the Mem0 MCP server), at the moment you learn it: decisions *and their rationale*, constraints/traps discovered the hard way, corrections to earlier assumptions (keep both the wrong answer and the right one), and requirement changes including what was ruled out and why. Prefer several specific memories over one long one — they surface independently on search. Batching memory to the end of a session is how it gets lost when the session is cut short.

9. **The repo is CODE ONLY — no content data, no profile-row exception.** Migrations carry schema only (`CREATE`/`ALTER`/index/constraint/function). No `INSERT` of content and no content-editing `UPDATE` for *any* table, including the `profile` identity row. UI chrome (labels, headings, nav, button copy) stays in code. Content lives in the live database; an idempotent snapshot `portfolio_seed_data.sql` is kept in a Google Drive folder named **`Portfolio`**. Refer to that folder by name only — never a URL. **Direction of truth:** edit in the admin console, then regenerate the snapshot from the live database (`core/db/gen_bootstrap_data.sql`). The snapshot is a bootstrap copy, not an authoring surface. A new feature ships DDL only; rows are added through admin, then the snapshot is regenerated. The repo alone cannot rebuild content — disaster recovery depends on the snapshot.

10. **Docker Compose is the local stack, and it stays paired with `.env.example`.** `docker compose up --build` is the same on Docker Desktop for Windows and for Mac. It starts Postgres (service `db`, user/password/database `portfolio`) and the app. `.env.example`'s `DATABASE_URL` is `postgres://portfolio:portfolio@db:5432/portfolio?sslmode=disable`. Host `db` and those credentials match `docker-compose.yml`. Compose mounts `LOCAL_CONTENT_DIR` (default `../portfolio-local-content`, forward slashes) at `/content`. `docker/local-admin.sql` is the placeholder admin and is not a migration. `.env.example` sets `GOOGLE_REDIRECT_URL=http://localhost:8080/login/google/callback` and `SITE_URL=http://localhost:8080`. A change to any of these updates `.env.example`, `README.md`, `docs/ARCHITECTURE.md`, and Mem0 in the same piece of work.

11. **Course and article content goes through one pipeline.** Every lesson/article upload passes `storage.NormalizeLesson` (site tokens, no in-lesson TOC, unnumbered `<h2>`, labelled code blocks; `<!-- vb:theme dark -->` for dark app-style lessons); offline edits use `go run ./cmd/lessonfmt`. Verify at 390 px and desktop in light and dark, keep the originals as a rollback copy (Storage `backup/{date}/` in the same bucket — copy back to roll back — or the Drive folder `Portfolio`), then clear `courses.outline` and flag the chatbot reindex. Full strategy and the owner's design references (UI UX Pro Max, Emil Kowalski, Watermelon UI, Brand-building skills) are in `docs/DESIGN.md` — read it before any redesign or content update.

## Before finishing any task in this repo

- Did this change make `docs/FUNCTIONALITY.md` or `docs/ARCHITECTURE.md` stale? Update it now.
- Did I reuse an existing helper/pattern instead of writing a near-duplicate?
- Does a new feature follow the models -> repo -> handlers -> templates -> routes layering as its own additive piece?
- Any new/changed `.sql` migration — is it safe to run twice? (See rule 4.)
- Any real secret in the diff? Check, don't assume.
- `go build ./... && go vet ./...` clean? Any changed template smoke-tested?
- If Docker, Compose, the local content directory, the local admin SQL, the local Google redirect URL, or the local `DATABASE_URL` changed, do `.env.example`, `README.md`, `docs/ARCHITECTURE.md`, and Mem0 still describe the same local stack?

Full detail and rationale live in `CLAUDE.md` and `.cursor/rules/architecture.mdc` — read those for the complete picture.

Also read `docs/DESIGN.md` (design tokens, motion rules, components) before touching anything visual.
