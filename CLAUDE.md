# Instructions for AI coding agents (Claude Code, Cursor, Codex, or any other)

This file is the entry point for Claude Code. Codex reads `AGENTS.md` and Cursor reads `.cursor/rules/architecture.mdc`/`.cursorrules` — all three carry the same rules, kept in sync on every push. Read this file, and the two docs it points to, **before making any change** — not after something breaks. If you're an AI agent and you're reading this mid-task having already edited files, stop, read `docs/FUNCTIONALITY.md` and `docs/ARCHITECTURE.md` now, then continue.

## Read these two docs first, always

- **`docs/FUNCTIONALITY.md`** — every feature, who can use it, the business rules behind each flow. Read this to understand *what the product does* before touching anything user-facing.
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

This isn't bureaucracy — it's what makes a new feature genuinely additive instead of a modification to existing code. A new paid product, a new admin CRUD section, a new public page: each of these should be a **new file** (or a small, clearly-scoped addition to an existing one) at each layer, not a change threaded through five unrelated files. Concretely: `core/repo/audit_requests.go` and `core/handlers/consultancy.go`'s `AuditCheckoutOrder`/`AuditCheckoutVerify` are the reference example of "a fourth paid product bolted on without touching the course or booking checkout code at all." Copy that shape for the next one.

**When extending something instead of adding something new**, prefer widening an existing struct/function over branching deep inside a handler with type switches or product-specific `if` chains — the three checkout flows (course, booking, audit) are intentionally *not* unified into one mega-function with branches, because that would make each one harder to reason about and higher-risk to change. Three small, parallel, copy-pasteable flows beat one large parameterized one here. Use judgment: this isn't a blanket "never share code" rule, it's "don't force-unify things that are conceptually separate just to avoid a few duplicated lines."

### 3. Design for horizontal scale, not vertical

This app runs as a single stateless serverless function (Vercel) that can have any number of concurrent cold/warm instances at once. That constrains what "correct" looks like:

- **No in-process state that must survive between requests**, except the DB connection pool itself (reused across warm invocations via `sync.Once` in `api/index.go` — see `docs/ARCHITECTURE.md` §9). No package-level mutable caches, no in-memory session store, no local file writes expected to persist — the filesystem is ephemeral and not shared across instances.
- **All persistent state goes through Postgres**, accessed via the pooled connection (`DATABASE_URL` must be the Transaction pooler string, not the direct connection — a burst of concurrent serverless invocations against a direct connection exhausts Postgres' connection limit fast).
- **Every DB round-trip on a hot path should be considered for concurrency.** `App.base()` runs on *every* page render and fetches three independent things concurrently via `sync.WaitGroup` for exactly this reason — a sequential chain of round-trips there multiplies across every single page view. When you add a new handler that needs multiple independent lookups, ask whether they can run concurrently the same way, and use `sync.WaitGroup` (best-effort, swallow individual errors) or `errgroup` (propagate the first error) depending on which behavior the feature actually needs — see the comment in `core/handlers/app.go` for why that choice isn't arbitrary.
- **Payment confirmation is never trusted from the client.** All three checkout flows verify the HMAC signature server-side before granting anything. A new paid flow must do the same — never flip a "paid" status from a client-reported success callback alone.

### 4. Never commit secrets — this is a hard rule, not a style preference

- `.env` and `.env.local` are gitignored — keep them that way. Real credentials (`DATABASE_URL` with a real password, `JWT_SECRET`, `RAZORPAY_KEY_SECRET`, OAuth client secrets, bot tokens) live **only** in Vercel's environment variables and a local, untracked `.env`.
- `.env.example` documents every variable with a placeholder or dummy value — never a real one, even a "harmless-looking" one from a test account. Before adding a new env var, add it to `.env.example` with a clear comment on what it's for and where to get it, matching the existing file's style.
- Before any commit or push, if you touched config, deployment, or anything credential-adjacent, re-check `git status`/`git diff` for anything that looks like a real secret — a connection string with an actual password, an API key that isn't obviously a placeholder — and stop to flag it rather than pushing it.
- Never log a secret, even at debug level, and never echo one into a commit message, a code comment, or this repo's own documentation.

### 5. Keep it consistent with what's already here

- Go 1.24, standard library `net/http`, no web framework — don't introduce one for a single feature.
- Server-rendered `html/template`, no client-side JS framework — don't add React/Vue/etc. for a single page.
- Plain CSS with the existing custom-property design tokens — no Tailwind/Bootstrap/CSS-in-JS.
- Raw SQL via `database/sql` — no ORM.
- rem units, not px, for anything visual (an accessibility fix made earlier in this project's history — don't regress it).

---

## Before every push (checklist)

1. Does this change make `docs/FUNCTIONALITY.md` or `docs/ARCHITECTURE.md` inaccurate? If yes, update the doc in this same commit.
2. Did I reuse an existing utility/pattern instead of writing a new one that does almost the same thing?
3. Does the new code follow the models → repo → handlers → templates → routes layering, as its own additive piece?
4. Is there anything resembling a real secret in the diff? (`git diff` it, don't assume.)
5. `go build ./... && go vet ./...` clean? CSS valid (no stray syntax errors)? Any new/changed template smoke-tested (a throwaway Go program calling `view.New()` + `Renderer.Render()` is the established pattern in this project's history — cheap, catches template errors before deploy)?

This file, `docs/FUNCTIONALITY.md`, and `docs/ARCHITECTURE.md` are meant to be read together, every session, by whichever agent — Claude Code, Cursor, or a human — picks up work on this repo next.
