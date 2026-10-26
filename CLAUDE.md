# Instructions for AI coding agents (Claude Code, Cursor, Codex, or any other)

This file is the entry point for Claude Code. Codex reads `AGENTS.md` and Cursor reads `.cursor/rules/architecture.mdc`/`.cursorrules` — all three carry the same rules, kept in sync on every push. Read this file, and the two docs it points to, **before making any change** — not after something breaks. If you're an AI agent and you're reading this mid-task having already edited files, stop, read `docs/FUNCTIONALITY.md` and `docs/ARCHITECTURE.md` now, then continue.

## Read these two docs first, always

- **`docs/FUNCTIONALITY.md`** — every feature, who can use it, the business rules behind each flow. Read this to understand *what the product does* before touching anything user-facing.
- **`docs/DESIGN.md`** — the design system: tokens, type, motion rules, components, lesson authoring conventions. Read this before touching anything visual; new UI reuses its tokens and classes.
- **`docs/ARCHITECTURE.md`** — package layout, the full ER diagram, auth model, payment flows, deployment. Read this to understand *how it's built* before touching anything structural.

They exist specifically so you don't have to reverse-engineer six months of decisions from the source tree alone. Treat them as more authoritative than your own guess about "how this probably works" — if the docs and your read of the code seem to disagree, that's a signal to look closer, not to trust the code and ignore the doc.

**Non-negotiable rule: update the relevant doc in the same commit as the change.** A new table → update the ER diagram in `ARCHITECTURE.md`. A new route or user-facing flow → update `FUNCTIONALITY.md`. A new package or a changed request-lifecycle detail → update `ARCHITECTURE.md`. This is not a follow-up task, not "when I have time" — it's part of finishing the change. A doc that's occasionally stale becomes a doc nobody trusts, and then it's worthless to the next agent (including a future you) who reads it. Before every push, ask: *did anything I changed make either doc inaccurate?* If yes, fix the doc before pushing, not after.

---

## Architecture rules

### 1. Reuse, don't reimplement

Before writing a new helper, formatter, or utility function, **search for one that already exists.** This codebase had a real bug from skipping this step: a template-only `money` formatting function was reimplemented independently of `models.FormatMoney`, and the two quietly disagreed on how a whole-dollar amount should render (`$3000.00` vs `$3000`). Fixed by deleting the duplicate and pointing the template func straight at the real one (`core/view/view.go`). That's the standard to hold every new utility to: one canonical implementation, referenced everywhere, never a second copy that can drift.

Concretely: money formatting → `models.FormatMoney`. Slug generation → `slugify()` in `core/handlers/admin.go`. Paragraph/HTML body rendering → `articleBody`/`splitLines` in `core/view/view.go`. Promo discount resolution → `resolvePromoCode()` in `core/handlers/promo.go`. Random token generation (OAuth CSRF state, access tokens, certificate codes, unusable placeholder passwords) → `authutil.RandomHex(n)` — this one was fixed in the same pass as the money bug: four call sites in `checkout.go`, `quiz.go`, `admin_users.go`, and `auth.go` each independently called `crypto/rand.Read` + `hex.EncodeToString`, consolidated into one helper. If you need something close to one of these but not quite, extend the existing function (add a parameter, add a branch) rather than forking it into a near-duplicate.

### 2. Follow the layered pattern — it's what makes this "plug-in-able"

Every existing feature touches these layers, in this order, and a new one should too:

**`models`** (struct + business-rule methods) → **`repo`** (one file per aggregate, *all* SQL lives here and nowhere else) → **`handlers`** (wires the repo into `App`, one handler per route) → **`view/templates/pages`** (the HTML) → **`routes.go`** (register the route).

This isn't bureaucracy — it's what makes a new feature genuinely additive instead of a modification to existing code. A new paid product, a new admin CRUD section, a new public page: each of these should be a **new file** (or a small, clearly-scoped addition to an existing one) at each layer, not a change threaded through five unrelated files. Concretely: `core/handlers/checkout.go`'s `CheckoutCreateOrder`/`CheckoutVerify` + the `purchases` table is the live reference implementation for a Razorpay create/verify pair in this codebase — copy that shape for the next paid product. (`core/repo/audit_requests.go`'s own checkout flow was removed; only its read-only `ListAll`, for the admin payments ledger, is still live — don't copy it as an example of a checkout flow.) `core/handlers/software_checkout.go`'s `SoftwareCheckoutOrder`/`SoftwareCheckoutVerify` + the `software`/`software_purchases` tables are the most recent example of this pattern applied to a genuinely new product (a fixed-price download, unlike a course's pay-what-you-want pricing) — a good second reference alongside checkout.go.

**When extending something instead of adding something new**, prefer widening an existing struct/function over branching deep inside a handler with type switches or product-specific `if` chains — the three checkout flows (course, booking, audit) are intentionally *not* unified into one mega-function with branches, because that would make each one harder to reason about and higher-risk to change. Three small, parallel, copy-pasteable flows beat one large parameterized one here. Use judgment: this isn't a blanket "never share code" rule, it's "don't force-unify things that are conceptually separate just to avoid a few duplicated lines."

### 3. Design for horizontal scale, not vertical

This app runs as a single stateless serverless function (Vercel) that can have any number of concurrent cold/warm instances at once. That constrains what "correct" looks like:

- **No in-process state that must survive between requests**, except the DB connection pool itself (reused across warm invocations via `sync.Once` in `api/index.go` — see `docs/ARCHITECTURE.md` §9). No package-level mutable caches, no in-memory session store, no local file writes expected to persist — the filesystem is ephemeral and not shared across instances.
- **All persistent state goes through Postgres**, accessed via the pooled connection (`DATABASE_URL` must be the Transaction pooler string, not the direct connection — a burst of concurrent serverless invocations against a direct connection exhausts Postgres' connection limit fast).
- **Every DB round-trip on a hot path should be considered for concurrency.** `App.base()` runs on *every* page render and fetches three independent things concurrently via `sync.WaitGroup` for exactly this reason — a sequential chain of round-trips there multiplies across every single page view. When you add a new handler that needs multiple independent lookups, ask whether they can run concurrently the same way, and use `sync.WaitGroup` (best-effort, swallow individual errors) or `errgroup` (propagate the first error) depending on which behavior the feature actually needs — see the comment in `core/handlers/app.go` for why that choice isn't arbitrary.
- **Payment confirmation is never trusted from the client.** All three checkout flows verify the HMAC signature server-side before granting anything. A new paid flow must do the same — never flip a "paid" status from a client-reported success callback alone.

### 4. Every migration must be idempotent — this caused a real outage

`core/db/db.go`'s `Migrate()` re-runs every un-recorded file in `core/db/migrations/` on **every cold start** (see rule 3 — Vercel can spin up any number of instances), and `Bootstrap()` treats a migration error as **fatal for the entire app**, not just the feature it belongs to. A migration that only works once will eventually run twice — a redeploy, a retried cold start, or (as actually happened) a migration applied manually to unblock work mid-session, then re-run automatically on the next deploy. That real incident: `ALTER TABLE ... ADD CONSTRAINT` was pushed as migration 034, but the constraint had already been added by hand a few minutes earlier via the Supabase MCP tool — the next cold start re-ran the file, hit `constraint already exists`, and took `www.vishnubalachandran.com` down entirely (`503: apply migration 034_...: relation already exists`) until a hotfix (`7601c8a`) landed.

- `CREATE TABLE IF NOT EXISTS` and `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` are naturally idempotent — Postgres supports the guard directly. Prefer these whenever possible.
- **`ALTER TABLE ... ADD CONSTRAINT` has no `IF NOT EXISTS` form in Postgres.** Wrap it:
  ```sql
  DO $$
  BEGIN
      IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'your_constraint_name') THEN
          ALTER TABLE t ADD CONSTRAINT your_constraint_name ...;
      END IF;
  END $$;
  ```
  Same principle for anything else without a native `IF NOT EXISTS`/`OR REPLACE` (some index types, some trigger syntax) — check first with a guard, don't assume the bare statement is safe to re-run.
- If you apply a migration manually (e.g. via an MCP tool, to unblock work before a deploy), the checked-in `.sql` file **must still be idempotent** — it will run for real on the next cold start once the code ships. Manually applying it is not a substitute for writing it correctly; it's exactly the scenario that causes this class of outage if the file isn't already safe to re-run.
- Test the idempotency claim, don't assume it: run the migration's exact SQL a second time (locally or via an MCP query tool against the dev/prod DB) and confirm it no-ops before pushing.

### 5. Never commit secrets — this is a hard rule, not a style preference

- `.env` and `.env.local` are gitignored — keep them that way. Real credentials (`DATABASE_URL` with a real password, `JWT_SECRET`, `RAZORPAY_KEY_SECRET`, OAuth client secrets, bot tokens) live **only** in Vercel's environment variables and a local, untracked `.env`.
- `.env.example` documents every variable with a placeholder or dummy value — never a real one, even a "harmless-looking" one from a test account. Before adding a new env var, add it to `.env.example` with a clear comment on what it's for and where to get it, matching the existing file's style.
- Before any commit or push, if you touched config, deployment, or anything credential-adjacent, re-check `git status`/`git diff` for anything that looks like a real secret — a connection string with an actual password, an API key that isn't obviously a placeholder — and stop to flag it rather than pushing it.
- Never log a secret, even at debug level, and never echo one into a commit message, a code comment, or this repo's own documentation.
- Third-party keys — even ones that end up public in page HTML, like the JDoodle embed keys — live only in Vercel environment variables. Never write their values into code, docs, commit messages, or Mem0.

### 6. Keep it consistent with what's already here

- Go 1.24, standard library `net/http`, no web framework — don't introduce one for a single feature.
- Server-rendered `html/template`, no client-side JS framework — don't add React/Vue/etc. for a single page.
- Plain CSS with the existing custom-property design tokens — no Tailwind/Bootstrap/CSS-in-JS.
- Raw SQL via `database/sql` — no ORM.
- rem units, not px, for anything visual (an accessibility fix made earlier in this project's history — don't regress it).

### 7. Keep project memory current — write it down as you go, not at the end

This repo is worked on by a rotating cast of agents that share no conversation history. `docs/FUNCTIONALITY.md` and `docs/ARCHITECTURE.md` cover *what the product is*; project memory covers *what was decided and what bit us* — the things a fresh agent would otherwise rediscover the expensive way.

Record in Mem0 (via the Mem0 MCP server) as it happens, not in a write-up at the end:

- **Decisions and their rationale.** Not just "we skipped the confirmation email" but why — no provider, no API key, no SPF/DKIM/DMARC. A decision without its reasoning gets silently reversed by the next agent.
- **Constraints and traps discovered the hard way.** The migration that caused the outage, the `ON CONFLICT` guard that turned out to be a no-op, the renderer that only emits raw HTML when the body starts with a tag. These are worth more than anything else in memory.
- **Corrections.** When a measurement or an assumption turns out to be wrong, record the correction *and* the original, so nobody re-derives the wrong answer from the same evidence.
- **Requirement changes**, including things explicitly ruled out and why.

Two rules of thumb. Write the memory at the moment the thing is learned — batching it to the end of a session is how it gets dropped when the session is cut short. And prefer several specific memories over one long one, because they surface independently on search.

Before concluding any non-trivial task, ask: *did I learn anything here that the next agent would want and could not get from the code?* If yes, it goes in Mem0 before you finish.

### 8. The repo is CODE ONLY — no content data, no profile-row exception

This repository holds code and schema. It does not hold the site's content, and there is **no exception for the `profile` identity row**. A fresh database built from the repo alone is a complete empty schema: no name, no bio, no skills, no articles. That is intentional.

**Stays in the repo:** schema DDL (`CREATE TABLE`, `ALTER TABLE`, indexes, constraints, functions), Go code, `html/template` files, CSS, JS, and all UI chrome — labels, headings, nav text, button copy, error strings. UI chrome is not content and must not be pushed into the database.

**Never in the repo:** every `INSERT` of content and every content-editing `UPDATE` — profile (including the identity row), articles, skill groups, experiences, education, course metadata, home features, availability rules, quiz questions, testimonials, certifications, publications, promo codes. These live in the live Postgres database, with an idempotent snapshot `portfolio_seed_data.sql` in a Google Drive folder named **`Portfolio`**.

**Refer to that folder by the name `Portfolio` only.** Never write a Drive URL, folder id, or file id into this repo — not in docs, not in comments, not in commit messages. An agent granted Drive access finds the folder by name through the Google Drive MCP server. An agent without Drive access cannot use a link anyway.

**Direction of truth — getting this backwards is the obvious trap.** Content is edited in the **admin console** against the live database. The Drive snapshot is then **regenerated from the live database** (`psql "$DATABASE_URL" -AtX -v ON_ERROR_STOP=1 -f core/db/gen_bootstrap_data.sql > portfolio_seed_data.sql`). The snapshot is a point-in-time copy and a bootstrap tool, not an authoring surface. Editing the script does not change the site.

When a new feature seems to need seed rows: it doesn't. Ship the DDL in a migration, add the rows through the admin console, regenerate the snapshot.

The repo alone cannot rebuild the site. Disaster recovery depends on the Drive snapshot (or a backup of the live database). That is a deliberate trade-off, not an oversight — see `docs/ARCHITECTURE.md`.

### 9. Local Docker Compose and `.env.example` stay paired

`docker compose up --build` is how this repo runs locally, on Docker Desktop for Windows and for Mac, from the same `docker-compose.yml`. `docker-compose.yml` starts Postgres (service `db`, user/password/database `portfolio`) and the app image from `Dockerfile`. `.env.example`'s `DATABASE_URL` is `postgres://portfolio:portfolio@db:5432/portfolio?sslmode=disable`. The host `db` is that service name. The user, password, and database name are the same values as `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`. Changing one without the others makes Compose fail at startup.

Compose bind-mounts `LOCAL_CONTENT_DIR` (default `../portfolio-local-content`, outside the repo; forward slashes if overridden) at `/content` and sets `LOCAL_CONTENT_DIR=/content` inside the app container. Files there use the Storage object paths: `articles/{slug}.html`, `courses/{slug}.html`, `resumes/default.pdf`, `resumes/{role-key}.pdf`. Production leaves `LOCAL_CONTENT_DIR` unset. `docker/local-admin.sql` inserts only the placeholder admin from `.env.example` and is not a migration. Apply it with `docker compose exec -T db psql -U portfolio -d portfolio -f /local-admin.sql` (Git Bash on Windows needs `MSYS_NO_PATHCONV=1` in front). `.env.example` sets `GOOGLE_REDIRECT_URL` to the local placeholder `http://localhost:8080/login/google/callback` and `SITE_URL` to `http://localhost:8080`. Google client id and secret stay empty until pasted. Shell scripts and the Compose file stay LF (`.gitattributes`); the image also strips CR from the entrypoint.

When any of those change — service name, credentials, published port, the image, the content directory, the local admin SQL, the local Google redirect URL, or the local `DATABASE_URL` — update `.env.example`, `README.md`, and `docs/ARCHITECTURE.md` in the same commit, and write the new values into Mem0. The next agent does not inherit this conversation.

### 10. Course and article content goes through one pipeline

Every lesson and article upload passes through `storage.NormalizeLesson` (`core/storage/normalize.go`): lesson palettes mapped onto the site tokens, in-lesson "On this page" blocks removed, `<h2>` section numbers stripped, code blocks labelled with their language. Dark app-style lessons declare `<!-- vb:theme dark -->`. Offline edits go through `go run ./cmd/lessonfmt -out DIR files...`. Before uploading, render each changed lesson and check it at 390 px and desktop in light and dark; keep the originals as a rollback copy (Storage `backup/{date}/` in the same bucket — copy back to roll back — or the Drive folder `Portfolio`); afterwards clear `courses.outline` for the changed courses and flag the chatbot reindex. The full strategy, and the owner's design references (UI UX Pro Max, Emil Kowalski, Watermelon UI, Brand-building skills, with links), live in `docs/DESIGN.md` — read it before any redesign or content update, and record decisions in Mem0.

### 12. Product rules that must not regress

Standing product rules (Oct 2026). Auth is stateless signed cookies (password + Google, security-question reset, `token_version` revocation, DB-backed throttling in `auth_throttle`); admin is a single bootstrap admin re-verified against the `admins` table. Course checkout is courses-only, fixed server-side price, email-only for guests (claim code or Google-verified email attaches a purchase), promo codes resolved by `resolvePromoCode`, amount never taken from the client, an optional contribution after a course verified by signature. Software stays login-only and issues Ed25519 licences (`core/licence`; private key only in the host env, never in the repo). Quiz: 5 non-overlapping sets per course bank, a hard 5-attempt cap enforced by a DB trigger, only an admin grants extras, correct answers never sent before submit; certificates are plain ASCII with a public verify page. Booking is public, no login, hourly 10:00-20:00 IST, no overlaps enforced by the DB, honeypot + throttle, private manage link. SEO titles/descriptions/keywords live in the database (`page_seo` and per-item columns, admin /admin/seo) with template fallbacks; renaming a course slug leaves a 301 in `slug_redirects`. The in-lesson compiler uses embed keys held only in the host env (`JDOODLE_EMBED_KEYS`); keys never appear in the repo, docs, page HTML or memory files. No course-specific content, prices, keys or personal data belong in this repo or its docs. Transactional email only: Resend for paid flows, Brevo for bookings and newsletter confirmations, admin BCC on every message, keys in Vercel env only. No marketing email without recorded consent. Password reset stays the security question.

### 13. **CI minutes are free here, so Actions are not restricted.** This repo is public, so GitHub Actions minutes are free and unlimited: workflows may run on any branch, PR or schedule. (GitHub itself only runs `schedule` triggers from the default branch, so the reindex poller runs from `main` regardless.) Deploys are Vercel's own Git integration, not Actions. If this repo ever becomes private, restore a main-only guard (`if: github.ref == 'refs/heads/main'` on each job) and a minutes budget, as the private Temple repo has.

---

## Before every push (checklist)

1. Does this change make `docs/FUNCTIONALITY.md` or `docs/ARCHITECTURE.md` inaccurate? If yes, update the doc in this same commit.
2. Did I reuse an existing utility/pattern instead of writing a new one that does almost the same thing?
3. Does the new code follow the models → repo → handlers → templates → routes layering, as its own additive piece?
4. **Any new/changed `.sql` migration — is it safe to run twice?** `CREATE ... IF NOT EXISTS` and `ADD COLUMN IF NOT EXISTS` are fine as-is; `ADD CONSTRAINT` and anything else without a native guard needs a `DO $$ ... IF NOT EXISTS ...` wrapper (see rule 4 above). This one caused a real production outage — don't skip it.
5. Is there anything resembling a real secret in the diff? (`git diff` it, don't assume.)
6. `go build ./... && go vet ./...` clean? CSS valid (no stray syntax errors)? Any new/changed template smoke-tested (a throwaway Go program calling `view.New()` + `Renderer.Render()` is the established pattern in this project's history — cheap, catches template errors before deploy)?
7. If Docker, Compose, the local content directory, the local admin SQL, the local Google redirect URL, or the local `DATABASE_URL` changed, do `.env.example`, `README.md`, `docs/ARCHITECTURE.md`, and Mem0 describe the same local stack?
8. Did any course or article content change? It goes through `storage.NormalizeLesson`/`cmd/lessonfmt`, is checked at 390 px and desktop in light and dark, and the originals are kept as a rollback copy (Storage `backup/{date}/` or the Drive folder `Portfolio`) (rule 10).

This file, `docs/FUNCTIONALITY.md`, and `docs/ARCHITECTURE.md` are meant to be read together, every session, by whichever agent — Claude Code, Cursor, or a human — picks up work on this repo next.
