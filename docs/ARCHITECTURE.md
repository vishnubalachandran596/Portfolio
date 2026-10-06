# Architecture Reference — vishnubalachandran.com

**Purpose of this document:** everything about *how* this codebase is built — stack, package layout, data model (with an ER diagram), request lifecycle, auth, payments, rendering, and deployment — detailed enough that a fresh AI agent session (or a new engineer) can start making correct changes without re-deriving any of this from scratch. Companion to `docs/FUNCTIONALITY.md`, which covers *what* the product does.

**Keep this updated.** A new table, a new package, a new route pattern, a new external integration — update this file in the same commit.

**Working conventions for any agent or developer touching this repo** live in `CLAUDE.md` (repo root), mirrored for Cursor in `.cursor/rules/architecture.mdc` and `.cursorrules`, and for Codex in `AGENTS.md` — read those before making a change, alongside this file and `docs/FUNCTIONALITY.md`.

---

## 1. Stack at a glance

| Layer | Choice | Notes |
|---|---|---|
| Language | Go 1.24 | Standard library `net/http` — no web framework |
| Routing | `http.ServeMux` (Go 1.22+ pattern syntax: `"GET /path/{id}"`) | Two muxes: public + admin, composed together |
| Templates | `html/template`, embedded via `go:embed` | Server-rendered, no client-side framework/build step |
| Database | PostgreSQL (Supabase-hosted) | Accessed via `database/sql`, driven by `jackc/pgx/v5`'s `stdlib` adapter (`stdlib.OpenDB` in `core/db/db.go`); `lib/pq` is a secondary dependency used only for its `pq.Array()` helper (e.g. `articles.tags text[]`), not as the connection driver. Raw SQL throughout, no ORM |
| Migrations | Custom runner, `core/db/db.go` | Numbered `.sql` files in `core/db/migrations/`, tracked in a `schema_migrations` table, applied on every cold start. These carry **schema only** — no content `INSERT` or content `UPDATE`, including the `profile` identity row. Content lives in the live database; an idempotent snapshot is kept in the Google Drive folder named `Portfolio` (see §11) |
| Auth | JWT in `HttpOnly` cookies (`golang-jwt/jwt/v5`) | Two independent sessions: admin and user (visitor) |
| Payments | Razorpay (Orders API + Checkout.js) | Server creates orders, verifies HMAC signatures server-side; India + international pricing |
| PDF generation | `github.com/go-pdf/fpdf` | In-process — resume, receipts, certificates; no headless-browser dependency |
| AI / RAG chatbot | NVIDIA NIM (free tier, `core/ai`) + pgvector | `nvidia/nemotron-3-embed-1b` embeddings, `openai/gpt-oss-20b` chat; vector store is the same Postgres instance, no separate vector-DB service — see §12 |
| Deployment | Vercel, one Go serverless function | `api/index.go` is the entrypoint; `vercel.json` rewrites everything to it |
| Static assets | `go:embed` (`web/embed.go`) | CSS/JS/images compiled into the binary, served at `/static/`, cache-busted via a content hash |
| CSS | Plain CSS, custom-property design tokens | No Tailwind/Bootstrap, no build step |
| JS | Vanilla JS, no framework | `main.js` (public), `admin.js` + `theme.js` (admin) |

---

## 2. Package layout

```
core/
  ai/           NVIDIA NIM client (free-tier embeddings + chat completion) for the RAG chatbot — see §12
  app/          Bootstrap() — the one place that wires config+DB+renderer into handlers.App
  authutil/     password hashing (bcrypt wrapper) + RandomHex, the one shared source of cryptographically random tokens (OAuth state, access tokens, cert codes, placeholder passwords)
  certificate/  PDF quiz-completion certificate generator
  config/       env-var loading (Config struct)
  storage/      private Supabase Storage client for lesson HTML, article HTML, role-resume PDFs, the profile photo, and the site logo
  db/           DB connection + migration runner
  geo/          visitor country detection from request headers
  handlers/     ALL HTTP handlers + App struct (the biggest package — see §4)
  middleware/   auth (admin+user sessions), domain/subdomain routing, security headers, logging
  models/       every domain struct + business-rule methods (pricing display, status enums, etc.)
  notify/       Telegram notification sender
  oauth/        Google OAuth2 client
  payments/     Razorpay client (order creation, signature verification)
  receipt/      PDF payment-receipt generator
  repo/         one file per table/aggregate — all SQL lives here, nowhere else
  resume/       PDF resume generator
  scheduler/    turns availability_rules + existing bookings into bookable time slots
  view/         html/template renderer setup + template FuncMap
web/
  embed.go      go:embed of static/ (CSS/JS/images) + the AssetVersion cache-busting hash
  static/       css/, js/, img/
  templates/
    layout/     base.html, admin_base.html, header.html, footer.html, adminnav.html
    pages/      one file per route (see docs/FUNCTIONALITY.md for what each renders)
api/
  index.go      Vercel serverless entrypoint (Handler func, warm-instance reuse via sync.Once)
cmd/server/
  main.go       local-dev entrypoint (plain http.ListenAndServe)
docs/
  FUNCTIONALITY.md   what the product does (companion to this file)
  ARCHITECTURE.md    this file
```

**Convention:** a new feature almost always touches the same four layers in the same order: **`models`** (struct + any business-rule methods) → **`repo`** (a new file, or new methods on an existing repo, all SQL contained here) → **`handlers`** (wire the repo into `App`, add a handler, render or redirect) → **`web/view/templates/pages`** (the HTML) → **`routes.go`** (register the route). Follow this order for anything new; it's what every existing feature does.

---

## 3. Data model — ER diagram

```mermaid
erDiagram
    users ||--o{ purchases : makes
    users ||--o{ bookings : makes
    users ||--o{ testimonials : submits
    users ||--o{ admin_messages : receives
    users ||--o{ quiz_attempts : takes
    courses ||--o{ purchases : "is bought via"
    courses ||--o{ quiz_questions : has
    courses ||--o{ quiz_attempts : has
    courses ||--o{ course_updates : "notifies buyers via"
    promo_codes ||--o{ promo_code_courses : scopes
    courses ||--o{ promo_code_courses : "scoped by"
    promo_codes ||--o{ purchases : "discounts (nullable FK)"
    promo_codes ||--o{ bookings : "discounts (nullable FK)"
    availability_rules ||--o{ bookings : "generates slots for (app-level, not FK)"
    articles ||--o{ article_thumbs_up : "receives"
    users ||--o{ article_thumbs_up : "casts (nullable FK — real login, guest-by-email, or a name-only vote with no user row)"
    articles ||--o{ article_contributions : "receives"
    users ||--o{ article_contributions : "pays (nullable FK — login or guest-by-email; same user may have many rows)"

    admins {
        serial id PK
        text email UK
        text password_hash
        text name
        timestamptz created_at
    }
    users {
        serial id PK
        text email UK
        text password_hash
        text name
        bool is_guest "true = captured via anonymous article thumbs-up form, never signed in"
        timestamptz created_at
    }
    profile {
        smallint id PK "always 1"
        text name
        text title
        text tagline
        text hero_subtitle
        text summary
        text location
        text email
        text linkedin_url
        text github_url
        text resume_url
        text photo_object "Storage path in profile-photo bucket, e.g. profile/photo.jpg; migration 071"
        text logo_object "Storage path in site-logo bucket, e.g. brand/logo.png; migration 071"
        text rag_context "admin-editable passage for the chatbot's RAG index, additive to summary; migration 073; see §12"
        smallint years_experience
        timestamptz updated_at
    }
    skill_groups {
        serial id PK
        text category UK
        text level
        text achievement
        text items "comma-joined tags"
        int sort_order
    }
    experiences {
        serial id PK
        text company
        text role
        text location
        text start_date
        text end_date
        text bullets "newline-joined"
        int sort_order
        note "UNIQUE (company, role, start_date)"
    }
    role_resumes {
        text role_key PK
        text label
        text object_path "path in the resumes bucket"
        int sort_order
        timestamptz created_at
    }
    education {
        serial id PK
        text institution
        text degree
        text field
        int start_year
        int end_year
        text details
        int sort_order
        note "UNIQUE (institution, degree, field)"
    }
    testimonials {
        bigserial id PK
        bigint user_id FK "nullable, NULL = admin-authored"
        text author_name
        text author_title
        text body
        bool is_published
        int sort_order
        timestamptz created_at
    }
    certifications {
        serial id PK
        text name
        text issuer
        date issued_on "nullable, NULL = no date on record"
        date expires_on "nullable, NULL = does not expire"
        text credential_url
        bool is_published
        int sort_order
        timestamptz created_at
        note "UNIQUE NULLS NOT DISTINCT (name, issuer, issued_on)"
    }
    publications {
        serial id PK
        text title
        text kind "free text: Conference talk, Podcast, …"
        text venue
        text url
        date published_on "nullable"
        text summary
        bool is_published
        int sort_order
        timestamptz created_at
        note "UNIQUE NULLS NOT DISTINCT (title, venue, published_on)"
    }
    case_studies {
        bigserial id PK
        text title
        text client_context "e.g. employer name — Vishnu's own, not third-party PII"
        text problem
        text approach
        text result
        bool is_published
        int sort_order
        timestamptz created_at
        note "UNIQUE (title); ships empty like certifications/publications — migration 060"
    }
    subscribers {
        bigserial id PK
        text email UK
        text source_page "home | courses | articles | other"
        timestamptz created_at
        note "pure visitor PII, never in the bootstrap-data snapshot — migration 061"
    }
    home_features {
        serial id PK
        text title UK
        text description
        int sort_order
    }
    courses {
        serial id PK
        text slug UK
        text title
        text summary
        text description
        text level
        text duration
        int price_cents "INR paise"
        text image_url
        bool is_published
        int sort_order
        bool is_paid
        int min_price_cents "INR paise, 0 = free -- courses sell in INR only since Oct 2026, migration 062"
        text content_key
        text content_html "legacy column, left empty"
        text content_object "private Storage path courses/{slug}.html"
        text rag_context "admin-editable passage for the AI chatbot's RAG index; migration 067; see §12"
        text[] tags "same shape as articles.tags; migration 072; powers the /courses topic filter"
        timestamptz created_at
        timestamptz updated_at
    }
    course_updates {
        bigserial id PK
        bigint course_id FK
        text note
        timestamptz created_at
        note "Postgres trigger fans this out to admin_messages for every paid buyer"
    }
    course_reviews {
        bigserial id PK
        bigint course_id FK
        bigint user_id FK
        int rating "1-5, CHECK constraint"
        text comment
        timestamptz created_at
        timestamptz updated_at
        note "UNIQUE(course_id, user_id); rating/comment are admin-only (GET /admin/courses/{id}/reviews) -- the public course page shows only the row count"
    }
    quiz_questions {
        bigserial id PK
        bigint course_id FK
        text question
        text option_a
        text option_b
        text option_c
        text option_d
        char correct_option "A-D"
        int sort_order
        note "UNIQUE (course_id, question)"
    }
    quiz_attempts {
        bigserial id PK
        bigint course_id FK
        bigint user_id FK "nullable"
        bool is_admin
        text name
        text email
        int total_questions
        int correct_answers
        int score_percent
        bool passed
        text certificate_code UK "nullable, set only when passed"
        timestamptz created_at
    }
    purchases {
        serial id PK
        bigint course_id FK
        bigint user_id FK "nullable column, but always populated since Oct 2026 — checkout requires a Google-authenticated account; ON DELETE SET NULL"
        text email
        int amount_cents
        text currency
        text status "created | paid | failed"
        text razorpay_order_id UK
        text razorpay_payment_id
        text access_token UK "bearer token unlocking /learn"
        bigint promo_code_id FK "nullable"
        text promo_code
        text promo_text "snapshot of the promo code's BannerText/Description at redemption time, for the receipt; migration 063"
        int discount_cents
        timestamptz created_at
        timestamptz paid_at
        note "CHECK (status <> paid OR paid_at IS NOT NULL)"
    }
    bookings {
        serial id PK
        bigint user_id FK
        text name
        text email
        text phone
        text message
        timestamptz slot_start UK
        timestamptz slot_end
        text status "pending_payment | confirmed | cancelled"
        int amount_cents
        text currency
        text razorpay_order_id "partial UNIQUE where <> ''"
        text razorpay_payment_id
        timestamptz previous_slot_start "nullable"
        timestamptz rescheduled_at "nullable"
        bool reschedule_acknowledged
        bigint promo_code_id FK "nullable"
        text promo_code
        int discount_cents
        timestamptz created_at
        note "amount_cents/currency/discount_cents/promo_code* are always 0/empty now — hourly consultancy has no payment step; razorpay_order_id is only a unique token for MarkPaid to scope its update to, never a real Razorpay reference"
    }
    audit_requests {
        bigserial id PK
        text name
        text email
        text company
        text brief
        int amount_cents
        text currency
        text status "pending_payment | paid"
        text razorpay_order_id UK
        text razorpay_payment_id
        timestamptz created_at
        timestamptz paid_at "nullable"
        note "Architecture Review — read-only now, historical rows only, self-checkout removed"
    }
    contact_messages {
        bigserial id PK
        text name
        text email
        text phone "optional"
        text message
        bool is_read
        timestamptz created_at
        note "one /contact form submission — no login, no FK, standalone lead capture"
    }
    availability_rules {
        serial id PK
        smallint weekday "0-6"
        time start_time
        time end_time
        int slot_minutes
        bool active
        timestamptz created_at
        note "UNIQUE (weekday, start_time, end_time)"
    }
    articles {
        serial id PK
        text slug UK
        text title
        text summary
        text body "the post itself — plain text or HTML; empty when content_object is set"
        text content_object "private Storage path articles/{slug}.html; empty means body is the post"
        text cover_image_url
        text_array tags
        int thumbs_up_count
        date published_date "nullable"
        bool is_published
        int sort_order
        timestamptz created_at
    }
    article_thumbs_up {
        bigserial id PK
        bigint article_id FK
        bigint user_id FK "nullable — set for a real or guest-by-email vote; UNIQUE(article_id, user_id) is the de-dup"
        text guest_name "only used when user_id is null (name given, no email) — never deduped"
        timestamptz created_at
    }
    promo_codes {
        bigserial id PK
        text code UK
        text description
        text banner_text "optional override for the public banner's copy; migration 063"
        text product "course | consultancy"
        int discount_percent
        int max_redemptions "0 = unlimited, total across every account"
        int max_redemptions_per_user "0 = unlimited; configurable per code, migration 062"
        int redemption_count
        timestamptz starts_at "migration 062"
        timestamptz expires_at
        bool is_active
        bool is_public "scrolls in the sitewide promo banner; migration 062"
        text allowed_emails "comma-separated; non-empty makes the code private/user-scoped and forces is_public false; migration 062"
        timestamptz created_at
    }
    promo_code_courses {
        bigint promo_code_id PK_FK
        bigint course_id PK_FK
    }
    admin_messages {
        bigserial id PK
        bigint user_id FK
        text body
        bool is_read
        timestamptz created_at
    }
    schema_migrations {
        text version PK
        timestamptz applied_at
    }
    chatbot_chunks {
        bigserial id PK
        text source_type "profile|skill|experience|education|course|article|consultancy|case_study|certification|publication|testimonial; migration 065"
        bigint source_id
        text title
        text url
        text chunk_text
        bool is_gated "paid course lesson content; migration 066; see §12"
        vector embedding "vector(2048), nvidia/nemotron-3-embed-1b; see §12"
        timestamptz created_at
    }
    chatbot_rate_limits {
        text session_id PK
        date day PK
        int message_count "30/day cap for POST /chat; migration 065"
    }
    chatbot_answer_cache {
        bigserial id PK
        text question_text
        text answer_text
        text[] source_titles "paired by position with source_urls"
        text[] source_urls
        bigint course_id "NULL = public; else gated to that course's buyers; migration 074; see §12"
        vector embedding "vector(2048)"
        int hit_count
        timestamptz created_at
        timestamptz last_served_at
    }
```

**Notes on the model that aren't obvious from the diagram:**
- `admins` and `users` are **completely separate tables** — an admin logging in does not create a `users` row, and vice versa. The two sessions (§5) are independent; the same email *can* hold both.
- `purchases.access_token` is the single mechanism gating `/courses/{slug}/learn` — see `courseAccess()` in `core/handlers/checkout.go`. A `$0` "free" purchase still gets a real row and a real token; it just skips Razorpay entirely (`amountCents == 0` branch in `CheckoutCreateOrder`).
- `audit_requests` is now read-only from the app's perspective — nothing writes new rows anymore since the Architecture Review's self-checkout was removed (see §6). The table and its historical rows are kept purely so the admin payments ledger still shows what was actually paid before the removal.
- `course_updates` has no Go-side fan-out logic — a **Postgres trigger** (`trg_course_updates_notify`, from migration `026_course_updates.sql`) does the INSERT into `admin_messages` for every buyer automatically. Inserting a `course_updates` row (from the admin course-edit form's "what changed" field, or directly via SQL) is the *entire* notification mechanism.
- `min_price_cents` on `courses` defaults to `0` at the column level (migration `030`), but every published course has carried a real, industry-level INR minimum (content, set via the admin course form) since Oct 2026 — the free-by-default "contribute as you like" state from Sept 2026 was retired in favor of real pricing plus whichever promo code is currently active as the free-access path. `CheckoutCreateOrder` (`core/handlers/checkout.go`) now requires a Google-authenticated session unconditionally — there is no guest-checkout branch left.
- **Multi-currency is retired (migration `062`, Oct 2026).** Every price on this site is INR only — `models.CurrencyINR`. There is no more visitor-region detection (`core/geo` and `X-Vercel-IP-Country` are gone entirely), no `courses.currency` or `courses.min_price_cents_inr` column, no `CHECKOUT_CURRENCY` env var, and no `Course.RegionalPrice`/`RegionalPriceDisplay` — `Course.PriceDisplay`/`MinPriceDisplay` render a plain INR figure unconditionally. Article contributions and the consultancy reference figures (`AuditPriceDisplay`, `RetainerRangeDisplay`) are INR-only the same way. Razorpay is still created with `models.CurrencyINR` explicitly passed through, rather than relying on the account's default.
- **Promo codes got three new scoping dimensions in migration `062`, all configurable per code from `/admin/promo-codes`:** a per-account redemption cap (`max_redemptions_per_user`, 0 = unlimited, 1 by default — independent of `max_redemptions`, the old total-across-everyone cap, both enforced in `resolvePromoCode` via `PurchaseRepo.CountUserRedemptions`); a start time (`starts_at`) alongside the existing `expires_at`; and a public/private split — `is_public` codes scroll in the sitewide promo banner (`PromoCodeRepo.ListActivePublic`, fetched concurrently in `startBaseChrome` on every single page load and rendered by the `promo_banner.html` partial in `base.html`, just below the header, on every page), while a code with any `allowed_emails` listed (comma-separated text, parsed in the repo layer, not a join table — it's a short, rarely-edited list) is always private regardless of the `is_public` checkbox: `AdminPromoSave` forces `IsPublic = false` the moment any email is present. `PromoCode.AppliesToAllCourses` is gone — an empty `promo_code_courses` join for a code now means "applies to any course" by itself, with no separate flag that could drift from it.
- Natural-key uniqueness (migrations `050` and `054`): `skill_groups.category`, `experiences (company, role, start_date)`, `education (institution, degree, field)`, `home_features.title`, `availability_rules (weekday, start_time, end_time)`, `quiz_questions (course_id, question)`, `articles.slug` (promoted from the existing unique index), and `UNIQUE NULLS NOT DISTINCT` on `certifications (name, issuer, issued_on)` and `publications (title, venue, published_on)`. Duplicate rows are cleaned in the same migration before the constraint is added, so a second cold start is a no-op.
- RLS with zero policies covers every public table, including `certifications` and `publications` (migration `051`) and `role_resumes` (migration `058`). The app connects as the table owner and bypasses RLS; this only closes the PostgREST `anon`/`authenticated` door.

---

## 4. `core/handlers` — the App struct and request flow

`handlers.App` (in `core/handlers/app.go`) is the one struct every handler is a method on. It holds:
- `Cfg *config.Config`, `DB *sql.DB`, `View *view.Renderer`, `Auth *middleware.Auth`
- One repo field per aggregate: `Profile, Skills, Experience, Resumes, Education, Features, Courses, Bookings, Availability, Admins, Purchases, Users, Articles, PromoCodes, QuizQuestions, QuizAttempts, AdminMessages, CourseUpdates, Testimonials, Certifications, Publications, CaseStudies, AuditRequests, ArticleVotes, ArticleContributions, ContactMessages, Subscribers, CourseReviews, ChatbotChunks, ChatbotRateLimits`, plus `CourseStorage` for the private buckets
- `Razorpay *payments.Razorpay`, `Telegram *notify.Telegram`, `Google *oauth.Google`, `Nvidia *ai.Client`, `Timezone *time.Location`

Shared page chrome is assembled by `base` / `startBase` / `baseFrom` in `app.go`. `BaseData` carries site name, title, meta description, canonical URL, nav links, login/admin state, the footer's LinkedIn link, notifications, and `AssetVersion`. Every page-specific `*Data` struct embeds it.

Chrome lookups (the profile row, for the LinkedIn link; plus, for a signed-in visitor, user name and notifications) stay on their own `sync.WaitGroup` on purpose: that contract is "degrade quietly" — a missing LinkedIn link or an empty notification menu is a fine outcome for a page that isn't about either. Page-owned lookups use `errgroup` and fail the render. Folding chrome into the errgroup would silently promote a swallowed error into a 500; the inverse would swallow a missing About section.

Those chrome lookups used to run as a **second sequential wave** after each page's own `errgroup.Wait()`, and pages that already loaded `profile` fetched it again. On the public hot path the handler now calls `startBase` (or `startBaseWithProfile` when it already loads that row) *before* its own lookups and finishes with `baseFrom` after them, so chrome rides in the same round-trip wave. `base()` remains the one-shot form for admin pages, error pages, and form re-renders that have no wave of their own to join.

Anonymous `/about` is therefore one wave of eight errgroup lookups. Anonymous `/` is one wave of four. `/articles` is one wave of the filtered page (count + rows, themselves concurrent under an errgroup inside `ArticleRepo.ListPublishedFiltered`), the topic list, and chrome. List queries select `articleListColumns` — everything except `body`, which only the article's own page needs. `/courses` is the same three-way shape as `/articles` — `CourseRepo.ListPublishedFiltered` (filtered/sorted/paginated page, its own count+rows concurrent), `ListAllTags`, and the full unfiltered catalog that backs the always-complete "Where to start" tracks — plus chrome, all in one wave. `ListPublishedFiltered`'s query carries two `LEFT JOIN` aggregate subqueries, neither backed by a stored column on `courses`: `review_count` (from `course_reviews`, the only review signal ever shown publicly — never the rating value, see migration 064) and `enrolled_count` (`1 + COUNT(purchases WHERE status = 'paid')` per course — the `+1` is a display floor so a brand-new course never reads "0 enrolled"). Both are computed live on every request rather than maintained as incrementing counters, so neither can drift from what `course_reviews`/`purchases` actually contain, and `checkout.go` needed no changes to make an existing or new purchase count — it already does, automatically, the next time the query runs. `/resume.pdf` has no chrome. A role that has its own file is one lookup plus the Storage read. Every other request loads the default row (`role_key` `default`) and reads that object. With no default file stored it is one wave of four and generates a PDF. The Person schema's `knowsAbout` is derived from the skill groups the home page has already loaded, not a separately maintained keyword list.

`App.Routes()` (in `routes.go`) builds the full `http.Handler`: one public `ServeMux`, one admin-auth `ServeMux`, one `RequireAdmin`-wrapped protected `ServeMux` for the rest of `/admin/`, composed and wrapped in `middleware.Logging(middleware.SecurityHeaders(middleware.CanonicalHost(siteURL)(middleware.DomainRouter(domain)(mux))))`. Public utility and gated routes that can render HTML but are not Google landing pages (`/search`, `/login`, `/booking`, `/account`, admin, and gated course learn/quiz/certificate pages) are wrapped with the shared `noindex` helper rather than added to the sitemap. See `docs/FUNCTIONALITY.md` for what each route does; the exact route table (method + path + handler) is the source of truth in `routes.go` itself — don't let this doc's prose drift from it, read `routes.go` directly for the literal list.

Handler files, by area:
| File | Covers |
|---|---|
| `public.go` | Home (incl. subscribe-form data), About (incl. case studies), Resume, CoursesList (incl. subscribe-form data), CourseDetail, Sitemap-adjacent data |
| `checkout.go` | Course purchase order/verify, `courseAccess()` gate |
| `consultancy.go` | Consultancy page (tiers, FAQ, case studies, JSON-LD), Booking page/confirm (free, no payment gateway), Contact page/submit |
| `admin_contact.go` | Admin contact-messages list + mark-read |
| `admin_credentials.go` | Admin CRUD for Certifications and Speaking & Publications |
| `admin_case_studies.go` | Admin CRUD for Case Studies |
| `newsletter.go` | Public `POST /subscribe` (capture only, no confirmation email) + admin subscribers list/delete |
| `articles.go` | Public article list (incl. subscribe-form data)/detail/thumbs-up + Admin article CRUD |
| `account.go` | `/account` page + PDF receipts + testimonial submission |
| `booking_reschedule.go` | Both admin- and self-service booking reschedule flows |
| `learn.go` | Gated course content serving inside the site chrome + watermark injection |
| `quiz.go` | Quiz taking, scoring, certificate download |
| `course_reviews.go` | Verified-buyer review submit (private rating/comment) + its admin-only list view |
| `auth.go` | Login page, Google OAuth start/callback, logout |
| `legal.go` | Generic static legal-page handler factory |
| `seo.go` | `baseURL`/`pageURL` helpers, Sitemap, Robots, LLMsTxt, IndexNow key + submit, Search |
| `schema.go` | JSON-LD builders (Person/Website/Service on Home; ProfessionalService + FAQPage on Consultancy; Article; Course) |
| `admin.go` | Admin login, dashboard, course CRUD, bookings, purchases, availability, profile, preview-as-visitor |
| `media.go` | Image-upload validation (`readImageUpload`, `sniffImageExt`) shared by the profile photo and logo; public `GET /media/profile-photo` and `GET /media/logo` streaming handlers |
| `admin_content.go` | Admin CRUD for skills, experience, testimonials |
| `admin_users.go` | Admin user list/detail/promote/demote/message |
| `admin_quiz.go` | Admin CRUD for quiz questions |
| `admin_promo.go` | Admin CRUD for promo codes |
| `admin_payments.go` | The combined payments ledger (purchases + bookings + audit requests merged) |
| `promo.go` | `resolvePromoCode()` — the one shared discount-resolution function all three checkout flows call |
| `chatbot.go` | Public `POST /chat` (RAG answer) + `GET /ai-assistant` (`AIAssistantPage`) + `chatbotSessionID`; admin `/admin/chatbot` page + `/admin/chatbot/reindex` + content-gathering/chunking helpers — see §12 |

---

## 5. Auth — two independent JWT sessions

Both live in `core/middleware/auth.go`, sharing one `Auth` struct (one JWT secret, one TTL — 7 days) but **separate cookies and separate context keys**:

| | Admin | User (visitor) |
|---|---|---|
| Cookie name | `portfolio_admin_session` | `portfolio_session` |
| Claims | `{admin_id}` | `{user_id}` |
| Gate middleware | `Auth.RequireAdmin` | `Auth.RequireUser` |
| Unauthenticated redirect | `/admin/login` | `/login?next=<path>` |
| Context accessor | `middleware.AdminIDFromContext` | `middleware.UserIDFromContext` |
| Non-redirecting check | `Auth.IsAuthenticated(r)` | `Auth.IsUserAuthenticated(r)` |

**Visitor login is Google-only** — deliberately, per the comment at the top of `auth.go`: one "Sign in with Google" button serves both a regular visitor and the site owner (a Google account whose email matches an `admins` row also gets an admin session issued alongside the normal user session), so there's no separate password to set up or lose. `core/oauth/google.go` is a standard authorization-code OAuth2 flow: `GoogleLoginStart` builds the consent-screen URL with a random CSRF `state` stashed in a short-lived cookie, `GoogleLoginCallback` exchanges the code, fetches the Google profile, and creates-or-finds a `users` row by email before issuing the session. Admin login (`/admin/login`) is the only place email/password auth exists in this codebase, via `core/authutil` (bcrypt).

**Admin preview-as-visitor** is a third, lightweight state: a plain cookie flag (`preview_visitor`), not a session — `isPreviewingAsVisitor(r)` short-circuits `base()`'s `isAdmin` computation to `false` while the admin's real session cookie stays intact, so `/exit-preview` just clears the flag.

---

## 6. Payments — Razorpay is wired up for courses only

**Course purchases are the only paid flow left on this site.** Consultancy (Architecture Review, retainer, hourly) never touches a payment gateway at all — no code path, no dormant endpoint, nothing to re-enable by flipping a flag. This was a deliberate removal, not a disable: the site owner isn't taking payment integration further any time soon, and having a live-but-unlinked paid endpoint sitting in the background was considered worse than just deleting it. If self-checkout for consultancy is ever wanted again, it needs to be rebuilt — `git log` on `consultancy.go`/`booking.html` around the removal commit is the reference for what the Razorpay version looked like.

**Course purchase** (`POST /courses/{slug}/checkout/order` → `.../verify`, `purchases` table) follows: **create-order handler** (server resolves the true price, creates a Razorpay order, writes a `pending` DB row) → client opens Razorpay's Checkout.js modal with that order ID → **verify handler** (HMAC-verifies the callback server-side via `Razorpay.VerifySignature`, flips the DB row to paid, fires a Telegram notification, returns a redirect). Access is **only ever granted from the verify step**, never trusted from the client-side "success" callback alone. It **skips Razorpay entirely when the resolved amount is $0** — see §3.

**Consultancy hourly** (`POST /booking/confirm`, `bookings` table) is a plain "reserve the slot, confirm it" flow with no price and no payment step of any kind — `BookingConfirm` in `consultancy.go` calls `BookingRepo.CreatePending` then `MarkPaid` back to back, purely to reuse the existing slot-race protection (the unique constraint on `slot_start`), not because anything is actually "paid." A synthetic `razorpay_order_id` (`"free-"+token`) only exists so `MarkPaid` has something unambiguous to scope its update to.

**Architecture Review and Fractional Advisory** have no create/verify endpoints at all — both route to `/contact` from `consultancy.html`, to be scoped and invoiced outside this site. The `audit_requests` table and `AuditRequestRepo.ListAll` still exist, read-only, purely so the admin payments ledger (`admin_payments.go`) can keep showing the real historical Architecture Review purchases from when self-checkout briefly existed; nothing writes to that table anymore.

`core/payments/razorpay.go`: `Razorpay.Enabled()` is `false` whenever `RAZORPAY_KEY_ID`/`RAZORPAY_KEY_SECRET` aren't set — the course create-order handler checks this (except its own $0 branch, which doesn't need Razorpay at all) and returns `503` with a friendly message instead of erroring.

**To add a new fixed-price paid product**, the course purchase flow (`checkout.go`'s `CheckoutCreateOrder`/`CheckoutVerify` + `purchases` table) is now the only live reference implementation for a Razorpay create/verify pair in this codebase.

---

## 7. Templates / rendering (`core/view`)

`view.New()` parses every page template **twice-composed**: once against the public layout (`base.html` + `header.html` + `footer.html` + `skills_grid.html`) for public pages, once against the admin layout (`admin_base.html` + `adminnav.html`) for admin pages — two explicit lists in `view.go`, so **a new page template must be added to one of those two lists** or `Renderer.Render` will 500 with "template not found". `Renderer.Render(w, status, "page_name", data)` executes the matched layout's entrypoint template, passing `data` as `.`.

`publicLayout` also carries **shared component partials** — templates more than one page renders, kept in one place rather than copy-pasted. `promo_banner.html` (`{{define "promo_banner"}}`, reads `.PromoCodes` off whatever `BaseData` it's handed) is called exactly once, from `base.html` itself right after the header — so it renders on *every* public page, not per-page — and lists every currently active, public promo code (`PromoCodeRepo.ListActivePublic`, fetched concurrently in `startBaseChrome`); empty renders nothing. A pure-CSS marquee (`.promo-banner-track`, `@keyframes promo-banner-scroll` in `style.css`), disabled under `prefers-reduced-motion`. `skills_grid.html` (`{{define "skills_grid"}}`, taking a `[]models.SkillGroup` as its dot) is the first: Home's "What I bring" and About's "Skills" both call `{{template "skills_grid" .Skills}}`, which is what stops the two grids from drifting apart the way two copies of the same markup did before. Add a new shared component to that list too. Its interaction is CSS-first — see the `.skills-grid` comment block in `web/static/css/style.css`. There are **two display modes with different layout mechanics**, which is the one non-obvious thing about this component: collapsed, the card face shows category, level, and four chips (plus "and N more") and a revealed panel is `position: absolute` over its own card (achievement summary plus the full tag list), and being out of normal flow is exactly what stops the grid reflowing and keeps the summary out of the resting layout; expanded ("Expand all"), the `.is-expanded-all` modifier returns every panel to `position: static` so the cards size to their content and CSS Grid equalises each row. The second mode exists because the first cannot scale to nine simultaneous panels — nine absolutely-positioned boxes overlap their neighbours and overflow the grid. `main.js` owns the mode toggle, the per-card click/tap toggle (which it suppresses while in expanded mode, so `aria-expanded` never disagrees with what is on screen), and the live-region announcement.

**Page-to-page navigation stays a real, server-rendered multi-page app — no client-side router, no fetch-and-swap.** Clicking an internal link in `main.js` never calls `preventDefault()`; it only schedules `#page-loader`/`#page-loader-mark` (a route-progress bar + a small branded badge, `base.html`) to fade in ~60ms after the click, cleared on the new page's own `pageshow` — a purely cosmetic bridge over the normal browser navigation's network/server wait, not a background data fetch. `style.css` additionally opts into the browser-native **cross-document View Transitions API** (`@view-transition { navigation: auto; }`) — zero JS, zero framework, pure progressive enhancement (unsupported browsers, e.g. Firefox, just navigate with no transition, no broken behavior either way) — which cross-fades the outgoing/incoming page once the new document has already arrived and parsed. The two are complementary, not redundant: the loader covers the wait *before* the new page is ready; the view transition smooths the visual handoff *once* it is. Neither has any SEO bearing — URLs, status codes, meta/OG/JSON-LD, and crawlability are all unchanged; this is compositor-level visual polish applied after the fact, not a change to what gets served or when.

Custom template functions (`view.go`'s `funcs` map): `currentYear`, `formatDate`/`formatDateOptional`/`formatTime`/`formatDateTime`, `money`, `add1`, `splitLines` (blank-line paragraph splitting), `articleBody` (the HTML-or-plain-text blog body renderer — HTML only if the trimmed content *starts* with `<`, otherwise falls through to escaped `splitLines`-style paragraphs; see the doc comment in `view.go` for why prefix-only detection matters), and `rich` (`**bold**`/`*italic*` only — HTML-escapes the input first, then turns just those two marker patterns into `<strong>`/`<em>`, so there's no raw-HTML path to sanitize against; used on admin-authored free-text fields that aren't a full HTML document the way an uploaded article/lesson body is — profile tagline/hero subtitle/summary, skill achievements, experience bullets, education details, testimonial quotes, case study fields, publication summaries, course summary/description, and article summaries).

Every page's `*Data` struct embeds `handlers.BaseData` (site name, title, meta description, canonical URL, nav links, login/admin state, notifications, `AssetVersion`). A page that needs JSON-LD defines a `{{define "jsonld"}}` block that `base.html` calls via `{{block "jsonld" .}}{{end}}`.

---

## 8. Domain routing (`core/middleware/domain.go`, `canonical.go`)

The public canonical host is `www` (`SITE_URL`, currently `https://www.vishnubalachandran.com`). `CanonicalHost` 301-redirects the bare apex and the old `home.` landing host to that www origin with the same path; the www host itself is **never** redirected (a canonical URL that 3xxs is Google's "Page with redirect" indexing failure). Preview hosts (`*.vercel.app`) and local dev are served as-is. `vercel.json` repeats the apex/`home.` → www 301 at the edge so HTTP apex is not a two-step chain (HTTP→HTTPS apex, then apex→www).

Optional (`DOMAIN` env var) subdomain-per-page-group scheme: `about.`, `courses.`, `consultancy.`/`booking.`, `articles.`, `admin.` each own their path prefix on their own subdomain. Unmatched paths (home, search, legal, static) live on **www**, not `home.`. A request on www is always served — including page-group paths — so the live path-based site stays indexable. Apex and `home.` 301 to www; other page-group subdomains still 301 to the subdomain that owns the path. An unrecognized host is served as-is. `SubdomainForPath(path)` is the same lookup, exposed for building absolute URLs; `"home"` resolves to the www host in `pageURL`. With `DOMAIN` unset, `DomainRouter` is a no-op and `CanonicalHost` still consolidates apex/`home.` onto www whenever `SITE_URL` prefers www.

---

## 9. Deployment

- **Vercel**, one Go serverless function. `api/index.go`'s `Handler` calls `app.Bootstrap()` exactly once per cold start (`sync.Once`) and reuses the resulting `*handlers.App` (including its DB pool) across warm invocations — never re-bootstrapped per-request.
- **`app.Bootstrap()`** (`core/app/bootstrap.go`): loads `config.Load()` (fails fast if `DATABASE_URL`/`JWT_SECRET` are unset), opens the Postgres pool, **runs pending migrations** (`db.Migrate` — tracked in a `schema_migrations` table by filename, so it's safe to run on every cold start; only un-applied files execute), builds the `view.Renderer`, constructs `handlers.App`, and — if `ADMIN_BOOTSTRAP_EMAIL`/`ADMIN_BOOTSTRAP_PASSWORD` are set and no admin exists yet — creates the first admin account (a no-op once any admin exists, safe to leave the env vars set permanently).
- **Static assets and templates are compiled into the binary** via `go:embed` (`web/embed.go` for CSS/JS/images, `core/view/view.go`'s `templateFS` for HTML) — there is no filesystem read at runtime for any of this, which is what makes a single stateless serverless function workable. Paid lesson HTML is not in the binary. `core/storage` reads it from the private `course-content` bucket after `courseAccess` passes (`SUPABASE_SECRET_KEY`, or the anon key plus the dedicated storage-reader login) and `PrepareLesson` returns a base64 iframe document: the lesson body, the lesson's own `<style>` (copied out of `<head>` so labs, compilers, and animations still run), the site stylesheet, a small reset so the site-wide `ul { list-style: none }` and any collapsed `word-spacing` cannot flatten lesson prose, and the watermark. `/courses/{slug}/learn` renders the site chrome and loads that document in a 75vw × 75dvh `.html-renderer` iframe (`sandbox="allow-scripts"`, no `allow-same-origin`) so the parent cannot read the lesson DOM. The watermark paints only inside that pane. The same login reads article HTML from the private `articles` bucket (`articles/{slug}.html`) and role-resume PDFs from the private `resumes` bucket (`resumes/{role_key}.pdf`). The article page reads that object and renders it in the same pane (in-page, for indexing), with the watermark mounted on `#html-renderer` so it does not cover the site chrome. Uploaded article `<style>` is still stripped so a post cannot restyle the public chrome. When `LOCAL_CONTENT_DIR` is set, `storage.New` uses that directory for the same object paths and does not call Supabase. Production leaves the variable unset. Admin preview uses that same reader after `RequireAdmin`: `GET /admin/resumes/{role_key}` streams the stored PDF, `GET /admin/articles/{id}/preview` renders the stored post (or `articles.body` when no file is set) in the article page, and `GET /admin/courses/{id}/preview` writes the stored lesson HTML. The course preview does not run `PrepareLesson`. The public learn route still does. None of these responses include a Storage URL. A warm instance may keep that login's access token in memory; dropping it only causes the next Storage request to sign in again. The token is not a content cache.
- **Profile photo and site logo are Storage objects too, migration 071** — not a repo-embedded static file (an earlier incarnation of the profile photo was `web/static/img/vishnu-photo.jpg`; that approach was retired specifically because `/static/` is served `Cache-Control: immutable, max-age=31536000`, so swapping the file in place at the same URL risked a browser or CDN edge serving the old bytes for up to a year). `core/handlers/admin.go`'s `AdminProfileSave` accepts an optional `photo_file`/`logo_file` multipart upload, sniffs the real format by magic bytes (`sniffImageExt` in `core/handlers/media.go` — never trusts a client-supplied `Content-Type` or filename), rejects anything that isn't JPEG/PNG/WEBP, and writes it to the private `profile-photo` or `site-logo` bucket at a fixed path (`profile/photo.<ext>` / `brand/logo.<ext>`) via the same `CourseStorage.Put`/`Delete` the resume upload already uses — if a new upload's extension differs from the stored one (a PNG replacing a JPEG), the old-extension object is deleted so two files for the same image never coexist. `profile.photo_object`/`logo_object` record the resulting path; empty means nothing uploaded yet. `GET /media/profile-photo` and `GET /media/logo` (public, no auth — `core/handlers/media.go`) stream whichever object is currently set, with `Cache-Control: no-cache` rather than a long max-age, so a replacement takes effect on the very next request with no risk of repeating the static-file incident. Home, About, the header (`header.html`), and the public layout's first-visit splash and between-page loader marks (`base.html`'s `#splash-loader`/`#page-loader-mark`, both gated on the same `BaseData.HasLogo`) all load the image from these fixed routes, never a bucket URL or a DB-stored URL string — with no logo uploaded, both loader marks fall back to the plain "VB" text badge they always showed. The admin form shows a live preview (client-side `URL.createObjectURL` on file selection) next to whichever image is already stored.
- **Database**: production is Supabase-hosted Postgres. On Vercel, `DATABASE_URL` must point at Supabase's **pooled (Transaction pooler) connection string**, not the direct connection — a serverless function opening/closing many short-lived connections against the direct connection exhausts Postgres's connection limit fast. `db.Open` logs a warning when the host looks like Supabase's direct connection (`*.supabase.co` without `pooler` in the hostname) rather than failing, so a plain local Postgres still boots. Local Compose uses a direct connection on purpose: one long-lived `cmd/server` process, not a fleet of serverless instances.
- **Connection pool**: a Vercel Go instance serves one request at a time, so the pool only has to cover the widest concurrent fan-out inside a single request — About's eight page lookups plus the two chrome lookups a signed-in visitor adds, currently `widestRequestFanOut+1` = 11 for both `MaxOpenConns` and `MaxIdleConns`. Matching idle to the ceiling means a warm instance does not re-establish connections on the widest page. If the fan-out grows, this constant has to grow with it, or one concurrent wave quietly becomes two.
- **Migrations**: `db.Migrate` reads `schema_migrations` once per cold start into a set, then skips already-applied files in memory. The previous per-file `SELECT count(*)` grew by one round trip with every migration added.
- **Local dev**: `docker compose up --build` is the local stack on Docker Desktop for Windows and for Mac (same Compose file; host paths use forward slashes). `docker-compose.yml` starts Postgres 16 (`db`, user/password/database `portfolio`) and the image built from `Dockerfile` (`cmd/server`). Copy `.env.example` to `.env` first. `DATABASE_URL` there is `postgres://portfolio:portfolio@db:5432/portfolio?sslmode=disable`: host `db` is the Compose service name, and the user, password, and database match `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`. Those four have to change together. Compose publishes Postgres on host port `POSTGRES_PORT` (default `5432`) and the app on `PORT` (default `8080`). `cmd/server/main.go` calls the same `app.Bootstrap()` as Vercel and runs `http.ListenAndServe`. Running `go run ./cmd/server` on the host against that database uses `localhost` in place of `db`. A missing `profile` row is an empty profile, not a 500; **Admin → Profile** inserts it on save. Migrations still insert no content. `docker/local-admin.sql` is the local placeholder admin (`admin@example.com` / `change-this-password`) and is not part of `Migrate()`. Apply it after the schema exists, or leave `ADMIN_BOOTSTRAP_EMAIL` and `ADMIN_BOOTSTRAP_PASSWORD` set so the first start creates the same account when `admins` is empty. Compose bind-mounts `LOCAL_CONTENT_DIR` (default `../portfolio-local-content`, outside the repo) at `/content` and sets `LOCAL_CONTENT_DIR=/content` in the app container. Files there are `articles/{slug}.html`, `courses/{slug}.html`, `resumes/default.pdf`, and `resumes/{role-key}.pdf`. `.env.example` sets `SITE_URL=http://localhost:8080` and `GOOGLE_REDIRECT_URL=http://localhost:8080/login/google/callback` (a placeholder; register it on the OAuth client and paste the client id and secret). The entrypoint script is LF, and the image strips a Windows CR before it runs.

---

## 10. Security posture

- CSP + `X-Content-Type-Options: nosniff` + `X-Frame-Options: DENY` + `Referrer-Policy: strict-origin-when-cross-origin` on every response (`middleware.SecurityHeaders`). CSP allows `'unsafe-inline'` for script/style (templates and embedded course-content pages both rely on inline blocks) but restricts *external* origins to only Razorpay's checkout domain — fonts are self-hosted (below), so `style-src`/`font-src` need no external allowance at all. `frame-src` allows `'self'` and Razorpay Checkout.
- All payment confirmation is server-side HMAC verification (`Razorpay.VerifySignature`) — never trusts a client-reported "payment succeeded."
- Admin and user sessions are fully independent JWTs in `HttpOnly`, `SameSite=Lax` cookies, `Secure` in production.
- Course-content access is an opaque random 32-byte hex bearer token, not a guessable ID. The learn page is the site chrome; the lesson loads from a base64 payload in a unique-origin iframe, so the outer page source does not contain the lesson tags and the parent cannot read the lesson DOM. `vishnubalachandran.com` is drawn once, horizontally, centered in the pane (`core/storage/prepare.go`'s `WatermarkScript` — originally drawn three times along the pane diagonal; changed to a single centered line to read cleanly against the site's own type instead of cutting across it at an angle), in the site's font (`"Montserrat"`, matching `--font-sans`) and the same muted color token as the rest of the page's faint text (`--color-text-faint`, both light and dark values) — only inside the renderer; copied text is stamped with the viewer's identity.
- `/admin/*` is `noindex`ed at the HTTP-header level (`X-Robots-Tag`), not just via `robots.txt`.

---

## 11. Content vs schema — disaster recovery

The repo is **code only**. Migrations create tables, indexes, constraints and functions. They do not insert Vishnu's profile, skills, articles, courses, or anything else a visitor would read. UI chrome (headings, nav, button labels) stays in templates and handlers; that is not content.

**Live Postgres is the source of truth for content.** Edits happen in the admin console (`/admin/*`). After a content change, regenerate the snapshot from the live database:

```
psql "$DATABASE_URL" -AtX -v ON_ERROR_STOP=1 \
     -f core/db/gen_bootstrap_data.sql > portfolio_seed_data.sql
```

The output is stored in a Google Drive folder named **`Portfolio`**. Refer to that folder by name only — never record a Drive URL in this repo. The generator (`core/db/gen_bootstrap_data.sql`) stays in the repo because it is code; the snapshot does not.

**Direction of truth.** Edit in admin → regenerate the snapshot. The snapshot is a bootstrap copy, not an authoring surface. Editing the SQL file and expecting the site to change is backwards.

**Disaster-recovery trade-off, stated plainly.** Checking out this repo and running migrations produces a complete, empty schema and a site that renders with no name, no skills, no articles. The repo **cannot rebuild content**. Recovering a wiped database means: apply the migration chain, then load `portfolio_seed_data.sql` from the `Portfolio` Drive folder (or restore a database backup). The first admin account is created by `Bootstrap()` from `ADMIN_BOOTSTRAP_EMAIL` / `ADMIN_BOOTSTRAP_PASSWORD`, not from the snapshot — the snapshot never contains password hashes. Local Docker can also load `docker/local-admin.sql`, which inserts only the placeholder admin from `.env.example` (`admin@example.com` / `change-this-password`). That file is not the snapshot and is not a migration. The article section of the snapshot includes `content_object` so a restored row still points at `articles/{slug}.html`.

Paid lesson HTML is not in the snapshot either. `courses.content_object` is the path (`courses/{slug}.html`) inside the private Storage bucket `course-content`. Article HTML uploaded from the admin console is a separate private bucket, `articles` (`articles.content_object`, path `articles/{slug}.html`). Role-resume PDFs are a third private bucket, `resumes`. The default download is the row `role_key = default` at `resumes/default.pdf`. A role-specific file is any other row, at `resumes/{role_key}.pdf`. The bytes live only in those buckets. Restoring the SQL without the bucket objects leaves every learn page, any uploaded article, and any resume download with a path and no file. Posts that still have `articles.body` set and an empty `content_object` restore with the SQL alone. A missing default resume row falls back to the generated PDF. The profile photo and site logo are the same story: two more private buckets, `profile-photo` (`profile.photo_object`, path `profile/photo.{ext}`) and `site-logo` (`profile.logo_object`, path `brand/logo.{ext}`) — restoring the SQL without those bucket objects leaves the header's logo and the Home/About hero photo pointing at a path with nothing there, same as a missing course lesson. Migration 055 creates `course-content` and a policy for `course-content@vishnubalachandran.com` when the `storage` schema exists; it does not create that login or upload the files. Migration 056 adds `articles.content_object`. Migration 057 creates the `articles` bucket and the same login's policy on it. Migration 058 creates `role_resumes` (with row-level security and no content rows) and the `resumes` bucket with that same login's policy. Migration 071 creates `profile-photo` and `site-logo` the same way, drops the old `profile.photo_url` column (the repo-embedded-file approach it replaced), and adds `photo_object`/`logo_object`.

`Migrate()` only executes files that exist and are not yet recorded in `schema_migrations`. It never re-reads an already-applied file, and it never checks that a recorded filename still has a matching file on disk. Editing or deleting a historical migration is therefore a no-op against production once that version is recorded; it only changes what a fresh database gets.

---

## 12. AI chatbot — RAG over site content (`core/ai`, `core/repo/chatbot_chunks.go`, `core/handlers/chatbot.go`)

A floating widget on every page (`chatbot-widget`, gated on `BaseData.ChatbotEnabled`) answers visitor questions about Vishnu's background, courses, and articles using retrieval-augmented generation — not a canned FAQ bot, and no paid API dependency. "AI Assistant" is the name used throughout (widget label, panel header, the dedicated page below) — a deliberately generic, commonly-searched term rather than site-specific branding, chosen so the feature can pick up generic "AI assistant" search traffic too, not just branded searches.

**One shared widget partial, two shells (`templates/layout/chatbot_widget.html`).** The widget's markup used to live inline in `base.html`; it's now `{{define "chatbot_widget"}}`, parsed into every public page (added to `publicLayout` in `view.go`, same as `skills_grid.html`/`subscribe_form.html`) so both call sites stay byte-identical rather than drifting (CLAUDE.md rule 1). `base.html` renders it as the floating bubble on every page except one: `{{if not .ChatbotFullPage}}{{template "chatbot_widget" .}}{{end}}`. `AIAssistantPage` (`GET /ai-assistant`, `chatbot.go`) renders the *same* partial as that page's entire main content, with `BaseData.ChatbotFullPage = true`, inside the normal page chrome (`ai_assistant.html` uses the standard `base.html`/header/footer layout like any other page — this is a real page, not a popup or iframe). The template itself switches shells off that one bool: full-page mode drops the floating toggle button and the panel's `hidden` attribute (there's nothing to show/hide — the whole page already is the chat) and gets a `chatbot-widget--full` CSS class instead of fixed positioning. `main.js`'s event wiring needs no branching for this: `chatbotToggle`/`chatbotClose` are simply `null` in full-page mode (no toggle/close elements rendered), so those two `addEventListener` calls are wrapped in `if (chatbotToggle)`/`if (chatbotClose)` null checks, and the full-page chat auto-focuses its input on load instead (`if (!chatbotToggle) chatbotInput.focus()`). The floating panel's header also gets a small expand-icon link to `/ai-assistant` (omitted in full-page mode, where it would be redundant) — that's the "open full screen" affordance.

**Why a real page and not just a bigger popup.** A floating bubble's panel is JS-rendered chrome repeated identically on every page; it has no URL of its own, so it can't be linked to, bookmarked, or indexed. `/ai-assistant` is an ordinary server-rendered route with its own `<title>`/meta description (`AIAssistantPage`'s `a.base(...)` call) and is listed in `sitemap.xml` (`seo.go`'s `Sitemap`) and `llms.txt` (`LLMsTxt`), both conditionally on `a.Nvidia.Enabled()` — the same "absent until configured" gate the floating widget itself already uses, so neither file ever points at a chat that isn't actually there. It's also linked from every page's footer (`footer.html`, gated the same way) for ordinary internal-link discoverability, alongside the floating panel's own expand link. The page's on-page copy (`ai_assistant.html`'s `<h1>`/lead paragraph) and `AIAssistantPage`'s own `<title>`/meta description both spell out common search terms ("AI Assistant", "AI chatbot", "virtual assistant") alongside Vishnu's name, deliberately — a floating widget's label is invisible to a crawler that's never executed the page's JS, so the dedicated page's visible text is the only place this copy can actually earn search visibility.

**The expand click hands the conversation to the new page, not just the URL.** Clicking the floating panel's expand icon (`#chatbot-expand`) used to just navigate to `/ai-assistant` and lose whatever had been asked so far — a visitor mid-conversation would land on a fresh greeting. `main.js` now writes the in-memory `chatHistory` array to `sessionStorage` (key `chatbotTransferHistory`) in that link's `click` handler, a plain synchronous write before the browser's normal navigation for the link proceeds (no `preventDefault` needed). On load, every chatbot instance — floating or full-page — checks for that key, and if present: clears the panel's default greeting, replays each transferred turn as a real message bubble, sets `chatHistory` to the transferred array so the next question continues the conversation server-side too, and removes the key immediately (read-once, so a later reload of the same page doesn't replay it). This is deliberately scoped to only the expand action — ordinary navigation to any other page still resets the conversation, which stays the right default (per the comment above `chatHistory`'s own declaration): sessionStorage is a one-shot handoff, not a second conversation store, so it doesn't reopen the stateless-serverless question §9/CLAUDE.md rule 3 already settled for `chatHistory` itself.

**Surfacing the chatbot's own purchased-course access on the course pages themselves.** The chatbot could already answer a buyer's questions about a course they'd unlocked (see the gated-retrieval paragraph below) — visitors just had no way to know that from the course pages themselves. `course_detail.html` now shows a one-line note ("the AI Assistant knows this course's lessons...") inside the `{{if .HasAccess}}` block, right next to the receipt/quiz buttons, and `course_learn.html`'s toolbar gets an "Ask AI Assistant" button linking to `/ai-assistant` alongside the existing quiz link (`.lesson-toolbar-actions`, a small flex wrapper so the toolbar's `justify-content: space-between` layout still holds with two buttons on the right instead of one). Both are gated on `.ChatbotEnabled`, reusing `Nav.AIAssistant` (`app.go`'s `NavLinks`) rather than hardcoding the path a third time. No handler or backend change was needed for the capability itself — the per-visitor unlocked-course gating in `Chat`/`unlockedCourseIDs` already covers a buyer asking from `/ai-assistant` exactly as it would from the floating widget on the course page, since both call the identical `/chat` endpoint.

**Why pgvector instead of a separate vector-DB service.** Rule 3 in `CLAUDE.md` keeps this app's persistent state in one place: Postgres. Rather than adding a dedicated vector database (Pinecone, Weaviate, etc.) as a second external dependency, the `vector` extension (already available on the Supabase project, enabled idempotently in migration 065) is enabled on the same database and `chatbot_chunks.embedding` is a `vector(2048)` column (resized twice — see the incident note below). The corpus is small (a personal site's content — a few hundred chunks at most), so retrieval is a plain sequential scan with pgvector's `<=>` cosine-distance operator (`ORDER BY embedding <=> $1::vector LIMIT k`) — no ANN index (HNSW/ivfflat) needed.

**Why NVIDIA NIM.** The user asked for a real RAG chatbot "without using an external token" — clarified to mean no *paid* token. NVIDIA's build.nvidia.com (NIM) offers a free-tier, OpenAI-compatible API (`https://integrate.api.nvidia.com/v1`) for both chat completion and embeddings, with no other infrastructure to run. `core/ai.Client` wraps it: `Embed(ctx, texts, inputType)` (`inputType` is `"query"` for an incoming question, `"passage"` for an indexed chunk) and `Chat(ctx, systemPrompt, history, userMessage)` (non-streaming; `openai/gpt-oss-20b` by default; `history` is the prior turns of the conversation — see "Conversational context" below). `Client.Enabled()` mirrors `payments.Razorpay.Enabled()` — the chatbot widget, and the admin `/admin/chatbot` page's working state, are both driven off whether `NVIDIA_API_KEY` is set, never a separate feature flag.

**Chat model: `openai/gpt-oss-20b`, chosen for portability, not just availability.** NVIDIA's own nemotron-3.5-lightning worked but needed a NVIDIA-specific `chat_template_kwargs: {"thinking": false}` request field to suppress its internal chain-of-thought (without it, a RAG-length prompt exhausted the whole token budget on reasoning and `content` came back empty with `finish_reason: "length"`). `openai/gpt-oss-20b` is also a hybrid-reasoning model, but every provider hosting it (NVIDIA NIM, Groq, OpenRouter, Fireworks, ...) exposes the same shape: the reasoning trace comes back in a separate `reasoning_content` field on the message, never mixed into `content`, and verbosity is capped with the standard `reasoning_effort` field (`"low"` here) rather than a vendor-specific flag — so a future move off NVIDIA (e.g. to Groq, deferred per Mem0) would need the model name and base URL changed, not the request/response shape rewritten. If a future model swap reintroduces empty/truncated replies, the fix is the same family of knob — a separate `reasoning_content`/thinking field to read or suppress — not an HTTP error, so it won't show up as a 410/404 the way a retired model does.

**Incident: NVIDIA retires NIM-hosted models without code-level warning, and its own reference docs don't reliably reflect it (Oct 2026).** The original embedding model, `nvidia/nv-embed-v1` (4096-dim), was retired by NVIDIA on 2026-08-25 and started returning `HTTP 410 Gone` from `/v1/embeddings` — this went undetected until the first real production traffic after `NVIDIA_API_KEY` was configured, because `chatbot_chunks` had never held a successful index before that (every embed call since go-live had been silently failing in the one way nobody had tested: a live user asking a question). Two bugs stacked to hide it: the embedding failure itself, and `Chat`'s error paths calling the shared `a.serverError` helper, which writes a **plain-text** body — but `web/static/js/main.js`'s chat widget calls `response.json()` unconditionally on every response, so a plain-text 500 threw inside that parse and surfaced only a generic "Something went wrong reaching the chatbot," with the real NVIDIA error visible nowhere but Vercel's runtime logs. First fix attempt (migration 068) switched to `nvidia/nv-embedqa-e5-v5`, picked from NVIDIA's own `docs.api.nvidia.com` reference page, which described it as "ready for commercial use" with no deprecation notice -- **that page was stale**: the live API returned the identical 410, with the identical "end of life on 2026-08-25" date, because it's the same retired generation. The working fix (migration 069) is `nvidia/nemotron-3-embed-1b` (2048-dim native output), confirmed instead via `build.nvidia.com`'s live model listing cross-checked against the model's own page for a release date (2026-07-16, after the retirement) and the absence of a deprecation notice -- verified empirically against the real production endpoint, not trusted from docs alone. Also added a dedicated `chatServerError` helper in `chatbot.go` that always returns the widget's own JSON error shape instead of reusing `a.serverError`, so the next failure (of any kind) surfaces a real message in the browser instead of masking it. **Lesson for future maintenance:** NVIDIA's NIM catalog churns and its reference docs can lag a retirement by months; a `chatServerError` log line reading `"status 410"` or `"status 404"` for `/embeddings` or `/chat/completions` means the configured model was retired, not that the code regressed. Don't trust a single reference page — check the model's own `build.nvidia.com` page for a release date and the absence of a deprecation notice, update `core/ai.New`'s `embedModel`/`chatModel`, add a migration to resize `chatbot_chunks.embedding` if the dimension changed (safe to do destructively — see `AdminChatbotReindex`, a full `TRUNCATE`+rebuild, never an append), and verify against the live production endpoint before considering it fixed.

**Schema** (migrations 065, 066, 070, 074):
```
chatbot_chunks          -- id, source_type ('profile'|'skill'|'experience'|'education'|'course'|'article'
                            |'consultancy'|'case_study'|'certification'|'publication'|'testimonial'),
                            source_id, title, url, chunk_text, is_gated, embedding vector(2048), created_at
chatbot_rate_limits     -- session_id, day, message_count  (PK: session_id+day)
chatbot_reindex_pending -- id (boolean, always true -- singleton), requested_at (NULL = nothing pending)
chatbot_answer_cache    -- id, question_text, answer_text, source_titles[], source_urls[], course_id (NULL
                            = public), embedding vector(2048), hit_count, created_at, last_served_at
```
`is_gated` (migration 066) marks a chunk that only an unlocked visitor may be answered from -- a paid course's lesson body. It exists because the first version of this feature indexed every course's full lesson content (fetched the same way the gated `/learn` page does) with no access check at retrieval time, which made the chatbot a free side-channel around that course's paywall: anyone could ask it to quote paid lesson content verbatim. `gatherChatbotChunks` now sets `IsGated: course.IsPaid` on a course's lesson-body chunks specifically (never on its public metadata chunk -- title/summary/price/level stay answerable to everyone, same as the public course listing -- and never on an article's, Storage-fetched or not: articles have no paywall, so their content is answerable for every visitor). `ChatbotChunkRepo.Search` takes the asking visitor's own unlocked course IDs (`unlockedCourseIDs` in `chatbot.go`) and filters with `WHERE NOT is_gated OR source_id = ANY($unlockedCourseIDs)` -- a gated chunk for a course the asker hasn't bought simply never enters the retrieved context, so it can't leak into an answer or a quoted passage. `unlockedCourseIDs` is `Purchases.ListPaidByUserID`'s course IDs for a signed-in buyer, empty for a signed-out visitor, and mirrors `courseAccess()`'s own admin-preview bypass (`core/handlers/checkout.go`) for a signed-in admin not previewing as a visitor -- every course, so the site owner can ask the bot about any locked lesson while testing, exactly as they can already open that lesson directly. When a question's only strong match is a locked course's gated content, the asker isn't buying it through the bot: retrieval falls back to whatever's left in the allowed pool -- that course's own public summary, other courses, or articles -- the same way it would for any other question. The system prompt also tells the model explicitly that locked-lesson specifics are excluded on purpose, so it declines rather than guesses if asked to fill in a gap.
`chatbot_rate_limits` backs the public `/chat` endpoint's 30-questions/day cap. It's keyed by an anonymous per-browser session id (`cb_sid` cookie, generated via the existing `authutil.RandomHex` helper — reused, not reimplemented) rather than by login, since most visitors asking the chatbot something aren't signed in; and it lives in Postgres rather than in-process, for the same reason every other piece of app state does (§9/CLAUDE.md rule 3) — a serverless fleet has no shared memory between instances.
`chatbot_reindex_pending` (migration 070) is the one-row flag `markChatbotReindexPending` sets and `CronChatbotReindex` consumes — see the Auto-reindex and incident notes below for why this exists instead of reindexing inline on every save.

**Answer cache (`chatbot_answer_cache`, migration 074) — skip retrieval and the NVIDIA call entirely for a repeat question.** `Chat` embeds every incoming question regardless (needed for retrieval either way), but before running `ChatbotChunkRepo.Search` or calling `Client.Chat`, it checks that same embedding against `ChatbotAnswerCacheRepo.Find` with a cosine-distance cutoff (`chatbotAnswerCacheMaxDistance`, 0.08) far tighter than `chatbotRelevanceThreshold` — a cache hit has to mean "essentially the same question," not just "a related one," or two different questions with different correct answers could collide. On a hit, the cached `answer_text`/sources are returned immediately and `ChatbotAnswerCacheRepo.Touch` bumps `hit_count`/`last_served_at` — no `chatbot_chunks` query, no NVIDIA chat-completion call, both of which a repeated or near-duplicate question (the common "what courses do you offer," "do you do consulting," "tell me about your experience" traffic a public chatbot actually gets) would otherwise pay for every single time. Caching is scoped to **first-turn questions only** (`cacheable := len(history) == 0`): once a conversation has history, the same literal question text can have a different correct answer depending on what preceded it, so caching under the bare question would risk serving a later, unrelated asker's first message the wrong cached reply. The intent-gate's decline reply (below) is never cached either — it's cheap to regenerate and caching it would risk permanently declining a question that becomes answerable once new content is indexed.

`course_id` mirrors `chatbot_chunks.is_gated`'s reasoning, but as the actual course identity rather than a boolean, because a boolean alone would let a buyer of *any* course read a cached answer that was only ever supposed to be answerable to a buyer of one *specific* other course they never paid for. `NULL` means the cached answer used no gated content and is safe to serve to anyone; a real id means the answer was generated from that course's gated lesson content, and `Find`'s `WHERE course_id IS NULL OR course_id = ANY($unlockedCourseIDs)` is the exact same filter shape `Search` already applies to a live chunk — the cache inherits the paywall check rather than reimplementing it. When writing a fresh answer to the cache, `Chat` scans which `chunks` it actually used: zero gated chunks → `course_id = NULL`; gated chunks all from one course → tag with that course's id; gated chunks from *more than one* course (a buyer of two courses asking something that touches both) → skip caching that answer entirely rather than invent a multi-course cache key for a rare case. Every `RebuildChatbotIndex` run calls `ChatbotAnswerCacheRepo.TruncateAll` after replacing `chatbot_chunks` — edited content means a cached answer could now be citing something stale, wrong, or deleted, and wiping the whole cache (same "full replace, not incremental" philosophy as `chatbot_chunks` itself) is simpler and safer than trying to figure out which cached rows are still valid. `/admin/chatbot` shows how many answers are currently cached and how many times the cache has actually been hit since the last reindex, so the optimization's effect is visible rather than invisible plumbing.

**Indexing (`RebuildChatbotIndex`, triggered via the admin button, auto-reindex, or the API endpoint below).** A full replace, not an incremental update: `gatherChatbotChunks()` re-reads the profile, every skill group, every experience/education row, every published course (metadata plus the lesson HTML fetched from Storage via the existing `CourseStorage` client and stripped of tags), every published article (summary plus body, again from Storage when stored there), every published case study, certification, and publication, every published testimonial, and the Consultancy page's own tiers and FAQ — all of it *live content*, never anything hardcoded in the repo, per CLAUDE.md rule 8, except the consultancy tiers/FAQ themselves, which are Go/template-literal copy rather than DB rows (there's no `consultancy_tiers` or `consultancy_faq` table — see `consultancy.go`) and so are read directly from the same `consultancyFAQ()` function the page itself renders from, rather than a repo call, guaranteeing the chatbot's answer and the page's own accordion can never say different things. A course's metadata chunk appends `courses.rag_context` (migration 067) when an admin has set one — a passage written specifically to read well as retrieved evidence handed to an LLM (what the course covers, who it's for, outcomes) rather than for a visitor skimming the course card the way `summary`/`description` are. It's additive, not a replacement: the derived facts (level, duration, price) stay in the chunk verbatim so they can't drift the way a paraphrase could, and an empty `rag_context` just falls back to summary/description alone, same as before the column existed. The profile chunk gets the same treatment via `profile.rag_context` (migration 073, admin-editable on `/admin/profile` right below the About summary field): a passage written to be quoted by the LLM directly, not skimmed by a visitor, so it's the place to spell out specific keyword-level facts (years of experience broken down by area, named tools, concrete AI/GenAI work) that would read as an oddly dense list on the About page itself but are exactly what a RAG chatbot answering "how many years has he worked with X" needs verbatim. Saving the profile form already calls `markChatbotReindexPending` (unconditionally, since `AdminProfileSave` has no dirty-field tracking), so editing this field alone is enough to pick it up on the next cron tick with no separate trigger. Each source is split into ≤1000-character chunks (`chunkText`, breaking on sentence/paragraph boundaries), embedded in batches of 32 (`inputType="passage"`), and written in one transaction via `ChatbotChunkRepo.ReplaceAll` (`TRUNCATE` + re-insert — simpler and safer than diffing against the previous index for a corpus this size, and it's what clears out chunks for since-edited or deleted content). A course lesson's or article's body is capped at `chatbotMaxBodyChars` (15,000 characters, ~15 chunks) *before* chunking — see the incident note just below for why — and every course's/article's Storage read runs concurrently (`fetchStorageBodies`, bounded to 5 at a time) rather than one at a time, since they're independent lookups (§4/CLAUDE.md rule 3) and the whole rebuild now runs inside one HTTP request's time budget.

**Incident: one oversized lesson file alone pushed a synchronous reindex past the timeout (Oct 2026).** Fixing the background-goroutine bug below surfaced a second, unrelated problem: `courses/java-sse-ultimate-guide.html` is over 800KB — the other nine published courses' content objects range 32KB-136KB — and embedding every chunk of every course/article in full (no cap existed yet) produced well over a thousand chunks, pushing a single `RebuildChatbotIndex` past `vercel.json`'s 60-second `maxDuration` into a `FUNCTION_INVOCATION_TIMEOUT`. That's a worse failure mode than the goroutine bug it replaced: since every course/article save triggers a full reindex, *every single save would have started failing outright* the moment this ran. The fix is `chatbotMaxBodyChars` above, plus the larger embed batches and concurrent Storage reads — together they keep a full rebuild of the current corpus comfortably inside the time budget (measured at ~45-50 seconds against the real corpus). That still wasn't a complete answer on its own, since a save-triggered reindex at that latency is still a slow save; the pending-flag-plus-Cron design above is what actually removes it from the save path. The 800KB file itself is worth checking by hand (likely unintentional bloat — embedded base64 images or duplicated content are the usual causes) but wasn't edited as part of this fix, since changing course content isn't this change's job (CLAUDE.md rule 8) and the cap makes the chatbot correct regardless of what's in it.

**Auto-reindex on save and delete, via a pending flag + a scheduled poller, not inline.** Every admin save/delete handler whose content `gatherChatbotChunks` reads calls `markChatbotReindexPending(ctx)` right before redirecting: courses and articles (`AdminCourseSave`/`Delete`, `AdminArticleSave`/`Delete`), profile (`AdminProfileSave`), skills (`AdminSkillSave`/`Delete`), experience (`AdminExperienceSave`/`Delete`), case studies (`AdminCaseStudySave`/`Delete`), certifications (`AdminCertificationSave`/`Delete`), publications (`AdminPublicationSave`/`Delete`), and testimonials (`AdminTestimonialSave`/`Approve`/`Delete`) — each a single fast `UPDATE` against the one-row `chatbot_reindex_pending` table (migration 070), logging (not propagating) any failure, same "degrade quietly" contract the rest of the app's chrome lookups follow (§4), and a no-op entirely when `NVIDIA_API_KEY` isn't set. This adds essentially no latency to a save. Education has no admin save/delete handler at all yet (`a.Education.List` is read-only from the admin side today), so there's nothing to hook there; the consultancy tiers/FAQ are Go/template-literal copy with no save path either (see the Indexing paragraph above) and are picked up automatically on whatever reindex runs next. `CronChatbotReindex` (`GET /internal/chatbot/cron-reindex`) is what actually does the work later; on each call it runs `ChatbotChunkRepo.ConsumePendingReindex` (one atomic `UPDATE ... RETURNING`, so two overlapping calls can't both decide there's work to do) and only runs the actual `RebuildChatbotIndex` when that returns true — the common case, nothing pending, costs one cheap query and no NVIDIA call at all. A new or edited piece of content is therefore searchable by the chatbot within about five minutes of the save, not instantly — see the two incident notes below for why synchronous-on-save was tried first and had to be walked back, and the note just below for why a GitHub Actions workflow calls this endpoint on a timer rather than Vercel's own Cron Jobs. The manual "Rebuild Index" button on `/admin/chatbot` (`AdminChatbotReindex`, POST `/admin/chatbot/reindex`) still runs a full rebuild synchronously for when five minutes isn't good enough — a first build, recovery after an embedding-model swap (migrations 068/069's incident — a new model needs the *whole* corpus re-embedded, not just what changed since), or confirming it's current right now.

**Incident: a "fire-and-forget" background reindex never actually ran, then its synchronous replacement made every save slow (Oct 2026).** The first version of auto-reindex (and of the admin button and the agent-facing API endpoint below) started `RebuildChatbotIndex` in a detached goroutine and responded immediately, on the theory that a save/delete shouldn't have to wait on a full re-embed. It silently never completed: confirmed empirically by triggering the API endpoint repeatedly and watching `chatbot_chunks`'s row count and newest `created_at` not move for several minutes afterward, with no error ever logged either -- the goroutine simply never got scheduled again. The cause is Vercel's Go serverless runtime itself: it freezes a function's entire process the instant its handler returns the HTTP response, with no `waitUntil`-style hook for a Go handler to keep work alive past that point, and no guarantee the same warm container (the only place that goroutine's state lives) is reused for a later request. The first fix made every trigger run `RebuildChatbotIndex` synchronously before responding -- correct (it actually completed), but it meant every course/article save now took as long as a full reindex (made worse by the oversized-file incident below), which is the latency the pending-flag-plus-Cron design above exists to remove. **Lesson for future maintenance:** on this specific deployment shape (one catch-all Go function on Vercel), a bare `go func(){...}()` that outlives the handler it was started from is not a reliable background-job mechanism -- there is no cheap way to defer *arbitrary* work past the response here. The two patterns that actually work are: do the work before responding (what `AdminChatbotReindex`/`APIChatbotReindex`/`CronChatbotReindex` all do, each inside its own real request), or record that work is needed in the database and let a separately-invoked request (the GitHub Actions workflow below, in this case) pick it up — never a goroutine that outlives the request that spawned it.

**Incident: Vercel's own Cron Jobs can't run this on the Hobby plan, so a GitHub Actions workflow does (Oct 2026).** The first version of this design configured a Vercel Cron Job in `vercel.json`'s `"crons"` array at `*/5 * * * *`. Every deploy after that silently stopped working: Vercel's deploy validation rejected it outright (`"Hobby accounts are limited to daily cron jobs ... Upgrade to the Pro plan to unlock all Cron Jobs features"`), which doesn't show up as a runtime error or a failed-but-visible deployment in the usual places -- it blocks the deployment from being created at all, so `git push` appeared to succeed while production silently stayed on the previous commit for several pushes in a row. A once-daily cron would be Hobby-compliant but defeats the purpose (a save at 9am wouldn't be reflected until the next day). The fix: `.github/workflows/chatbot-reindex.yml` runs on GitHub's own free Actions scheduler every 5 minutes (`workflow_dispatch` also lets it be triggered by hand) and just calls `GET /internal/chatbot/cron-reindex` with `Authorization: Bearer ${{ secrets.CHATBOT_REINDEX_TOKEN }}` -- the endpoint itself didn't need to change at all, since it was never actually Vercel-specific code, just reached by a different scheduler. The GitHub Actions secret `CHATBOT_REINDEX_TOKEN` must be set on this repo (Settings → Secrets and variables → Actions) to the same value as the Vercel env var of the same name. **Lesson for future maintenance:** a Vercel platform feature's availability can depend on the account's plan tier in ways that fail at deploy time with no visible error on the deployment list -- if a push stops producing a new deployment with no explanation, check whether `vercel.json` changed and try a direct `create_deployment` call (or `vercel deploy` locally) to surface the actual validation error, rather than assuming the webhook is just slow.

**Agent-triggered reindex (`APIChatbotReindex`, POST `/api/chatbot/reindex`, public route, bearer-token-gated).** A coding agent that edits course or article content directly against Supabase — a DB row, or an HTML object in the `course-content`/`articles` Storage buckets — bypasses the admin UI's save/delete handlers entirely, so the auto-reindex above never fires for that change. This endpoint gives such an agent the same trigger without an admin browser session: `Authorization: Bearer <CHATBOT_REINDEX_TOKEN>` (`crypto/subtle.ConstantTimeCompare`, not `==`, to avoid a timing side-channel on the token) runs the identical `RebuildChatbotIndex` synchronously and responds `200` with the resulting chunk count once it's actually done (see the incident note above for why this doesn't respond `202`/"started" before the work finishes). The endpoint itself returns `404` whenever `CHATBOT_REINDEX_TOKEN` is unset — same "absent until configured" pattern as `NVIDIA_API_KEY` gating the chatbot widget — so there's no unauthenticated reindex trigger exposed by default on a fresh deploy.

**`CronChatbotReindex` (`GET /internal/chatbot/cron-reindex`) reuses the same token for a different caller: a scheduled job, not an agent.** It checks the incoming bearer token against `a.Cfg.ChatbotReindexToken` — the exact same comparison `APIChatbotReindex` already does — rather than adding a second secret for what is, functionally, the same permission ("you may trigger a reindex") granted to a different caller. `.github/workflows/chatbot-reindex.yml` (see the incident note above for why this is a GitHub Actions workflow and not a Vercel Cron Job) sends that same `CHATBOT_REINDEX_TOKEN` value, stored as a GitHub Actions secret of the same name, as its `Authorization` header every 5 minutes. Unlike `APIChatbotReindex`, this handler calls `ChatbotChunkRepo.ConsumePendingReindex` first and only runs `RebuildChatbotIndex` when that reports true -- an agent calling the API endpoint directly wants a rebuild *now, unconditionally*; the scheduled job's whole purpose is to do nothing on the common tick where nothing changed, so it doesn't spend NVIDIA free-tier budget on no-op runs every 5 minutes, 24/7.

**Answering a question (`Chat`, POST `/chat`, public).** Embed the question (`inputType="query"`) → `ChatbotChunkRepo.Search` for the 5 closest chunks, scoped to the asker's own unlocked courses → **intent/relevance gate** (below) → build a system prompt that includes only that retrieved text and instructs the model to say "I don't have that information" rather than invent facts, prices, or dates → `Client.Chat` → return the reply plus the deduplicated source URLs the retrieved chunks came from. The widget (`web/static/js/main.js`) renders the reply and clickable source links in a floating panel; its markup is entirely absent from the rendered page (not just hidden by CSS) when `ChatbotEnabled` is false, and it's present on every public page (including a gated lesson page itself, `course_learn.html`) since they all share the same `base.html` layout.

**Conversational context (client-held history, no server-side session store).** The widget keeps a plain in-memory JS array, `chatHistory`, of `{role, content}` turns for the lifetime of the page (`web/static/js/main.js`) and sends it as `history` in every `/chat` request body, alongside the new `message`. There is deliberately no server-side conversation store — per CLAUDE.md rule 3, this app has no in-process state that survives between requests and a serverless fleet has no shared memory between instances, so a session-keyed history table would need the same kind of plumbing `chatbot_rate_limits` already has just to hold what the browser can hold for free. The server still never trusts client-supplied history blindly: `sanitizeHistory` (`chatbot.go`) drops any turn whose `role` isn't `"user"`/`"assistant"`, trims and caps each turn's content at `chatbotMaxHistoryChars` (500), and keeps only the most recent `chatbotMaxHistoryTurns` (6) turns — bounding both the prompt size and the cost of a pathological client payload. This fixed a real reported failure: a visitor asked a follow-up ("As lead?") after an initial question about AI experience, and the bot — having no memory of the prior turn at all — answered as if "As lead?" were a standalone, context-free question and fell back to its "I don't have that information" default. Two places needed the history, for different reasons: `historySearchQuery` folds only the most recent *user* turn (never the assistant's reply, never older turns) into the text that gets embedded for retrieval — enough to disambiguate a short follow-up like "As lead?" into something `Search` can match against, without diluting the embedding with unrelated older context; the full sanitized history, separately, is passed to `Client.Chat`'s `messages` array so the model actually sees the conversation when generating its answer. `Client.Chat` (`core/ai/nvidia.go`) builds `[system, ...history, user]` before marshaling — the system prompt's own text was extended to tell the model the conversation is continuous, so it should resolve a short follow-up against the prior turn rather than treating every message as the first.

**Intent gate before the generation call.** `ChatbotChunkRepo.Search` returns each chunk's pgvector cosine distance from the query (0 = identical, 2 = opposite). If the single closest chunk's distance exceeds `chatbotRelevanceThreshold` (0.75, hand-picked, not measured against a labeled set -- revisit if real traffic shows it too strict or too loose), `Chat` returns a fixed "I can only answer questions about Vishnu..." reply and returns *before* calling `Client.Chat` at all. This is deliberately not a second LLM call: the embedding + retrieval step already has to run to answer any in-scope question, so the distance it produces is free signal, and skipping the context-stuffed generation call on an off-topic question is where the actual token cost lives. A plainly off-topic question (general trivia, something unrelated to Vishnu/his courses/his articles) never reaches the LLM.

---

*Last updated: October 2026, alongside the RAG chatbot and AI/GenAI course work. Companion doc: `docs/FUNCTIONALITY.md`. If you're an AI agent about to modify this codebase, read both files first — they're written specifically so you don't have to re-derive this context from the source tree.*
