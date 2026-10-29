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

**Licence keys (Temple Accounts, `core/licence`, `core/handlers/licence.go`, `core/repo/licences.go`, migration `084`).** The desktop app verifies a key offline with an Ed25519 PUBLIC key; this site signs with the matching PRIVATE key, which lives only in the Vercel env secret `LICENCE_SIGNING_KEY_B64` (base64 of the 32-byte seed) with `LICENCE_KID` its id. The public key (and the same `kid`) goes in the Temple repo's `config/licence_keys.json`. Generate the pair in the admin console at `/admin/licences/keygen` (shown once, `Cache-Control: no-store`, never stored or logged, POST only so `SameOrigin` applies). Key format, fixed by the app: payload = compact JSON, keys sorted, exactly `{"dev","iat","kid","lic","prod","v":1,"ver"}` (no `updates_until`); signature = Ed25519 over `b"vb-licence/v1\n" + payload`; key = `"VBL1-" + b64url(payload) + "." + b64url(sig)` without padding. `core/licence/licence_test.go` pins the app owner's published test vector (throwaway key, test only), so any drift in serialisation fails the build. Every paid software purchase (and every free/promo checkout) creates exactly one `licences` row (`EnsureForPurchase`, idempotent through the partial unique index on `purchase_id`), only for software whose `licence_product` is set; two purchases give two licences; an admin-added licence has no purchase. That is how a buyer gets a licence for another device: `software_purchases` has no unique key on `(user_id, software_id)` (none was ever added, so no migration was needed) and `SoftwareCheckoutOrder` has no already-purchased guard, so My licences' *Get a new licence* box simply reposts to the same `POST /software/{slug}/checkout/order` and `.../verify` pair (the shared `[data-software-buy]` handler in `main.js`). `softwareLanding` (`software_checkout.go`) sends an extra purchase (`SoftwarePurchaseRepo.CountEarlierPaid` > 0, ordered by id so a replayed verify stays stable) to `/account/licences?new=1#lic-{id}` (`LicenceRepo.GetByPurchase`); `extraPurchaseIDs` labels later purchases "extra licence" in the admin ledger and `/account`. Do not add a unique index on `(user_id, software_id)`. The owner opens `/account/licences`, pastes the full 64-hex Device ID (spaces and hyphens ignored, any hex length accepted); the signed key stays on the page with a `.lic` download. Moving a licence to another computer is limited to 2 self-service changes per rolling 12 months (counted from `licence_events`); the admin console can reissue to any device (which also clears a revoke). A revoke cannot be enforced offline, so reissue is the only remedy. The old "MAC address + shared secret" scheme is gone; nothing in this repo derives keys from a shared secret. Migration `092` drops `software.licence_versions` and `licences.ver` (`DROP COLUMN IF EXISTS`, existing rows intact) and adds nullable `software.microsoft_store_url`. The per-OS downloads block is the shared `software_downloads` partial (`layout/software_downloads.html`), rendered on `software_detail` and `purchase_success` for items with a `licence_product`; only the Store URL is data. `/temple-accounts/privacy` is a `LegalPage` slug. The promo banner's scope tooltip is filled by `PromoCodeRepo.ListActivePublic` through JSON sub-selects in its one query (`PromoCode.ScopeItems`), so the banner still costs one round trip.

---

## 2. Package layout

```
core/
  ai/           NVIDIA NIM client (free-tier embeddings + chat completion) for the RAG chatbot — see §12
  app/          Bootstrap() — the one place that wires config+DB+renderer into handlers.App
  licence/      Ed25519 licence-key signer (Temple Accounts): payload, signing, Device ID normalisation, key-pair generation. Pure functions, no I/O
  authutil/     visitor password hashing (bcrypt wrapper), SecretEquals (constant-time secret compare, used for the admin password) + RandomHex, the one shared source of cryptographically random tokens (OAuth state, access tokens, cert codes, placeholder passwords)
  certificate/  PDF certificate generator, ASCII sanitising rule (ascii.go), code helpers
  config/       env-var loading (Config struct)
  storage/      private Supabase Storage client for lesson HTML, article HTML, role-resume PDFs, the profile photo, and the site logo
  db/           DB connection + migration runner
  geo/          visitor country detection from request headers
  handlers/     ALL HTTP handlers + App struct (the biggest package — see §4)
  middleware/   auth (admin+user sessions), domain/subdomain routing, legacy/trailing-slash redirects, security headers, logging
  models/       every domain struct + business-rule methods (pricing display, status enums, etc.)
  notify/       Telegram alerts, plus Resend (orders and contributions, including a zero amount) and Brevo (bookings, consultancy connect requests, and newsletter confirmations)
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
    users ||--o{ software_purchases : makes
    users ||--o{ bookings : makes
    users ||--o{ testimonials : submits
    users ||--o{ admin_messages : receives
    users ||--o{ quiz_attempts : takes
    courses ||--o{ purchases : "is bought via"
    courses ||--o{ quiz_questions : has
    courses ||--o{ quiz_attempts : has
    users ||--o{ quiz_attempt_grants : granted
    courses ||--o{ quiz_attempt_grants : for
    courses ||--o{ course_updates : "notifies buyers via"
    promo_codes ||--o{ promo_code_courses : scopes
    promo_codes ||--o{ promo_code_software : scopes
    software ||--o{ promo_code_software : "scoped by"
    courses ||--o{ promo_code_courses : "scoped by"
    promo_codes ||--o{ purchases : "discounts (nullable FK)"
    promo_codes ||--o{ bookings : "discounts (nullable FK)"
    software ||--o{ software_purchases : "is bought via"
    promo_codes ||--o{ software_purchases : "discounts (nullable FK)"
    availability_rules ||--o{ bookings : "generates slots for (app-level, not FK)"
    articles ||--o{ article_thumbs_up : "receives"
    users ||--o{ article_thumbs_up : "casts (nullable FK — real login, guest-by-email, or a name-only vote with no user row)"
    articles ||--o{ article_contributions : "receives"
    users ||--o{ article_contributions : "pays (nullable FK — login or guest-by-email; same user may have many rows)"

    admins {
        serial id PK
        text email UK "the admin list; no password column (migration 080)"
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
        int token_version "in the session cookie; bump = sign out everywhere"
        bool password_usable "true only for a user-chosen password (Google accounts keep a random unusable hash)"
        bool google_verified "Google has proven control of the email"
        text security_question "one of the fixed list"
        text security_answer_hash "bcrypt of the normalised answer"
    }
    page_seo {
        text path PK "static page path: /, /about, /courses, ... (models.StaticSEOPages)"
        text seo_title "empty = built-in title"
        text meta_description "empty = built-in description"
        text primary_keyword "author reference, never rendered"
        text og_image "absolute URL or site path; empty = site default"
        bool noindex "meta robots noindex, follow + dropped from the sitemap"
        timestamptz updated_at
        note "migration 085; RLS enabled; rows hold overrides only, never committed seed copy"
    }
    auth_throttle {
        text kind PK "login | reset | signup | admin"
        text key PK "SHA-256 of lower-cased email or client IP"
        int fail_count
        timestamptz last_fail_at
        timestamptz locked_until
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
    email_outbox {
        bigserial id PK
        text kind "purchase_course | purchase_software | purchase_article | contribution | booking_* | newsletter_confirm | send_test"
        text entity_id "purchase, software purchase, article contribution, course contribution, booking, or subscriber id"
        text to_email
        text provider "resend | brevo | disabled"
        text status "pending | sent | failed | disabled"
        int attempts
        text last_error
        text provider_id
        timestamptz created_at
        timestamptz updated_at
        timestamptz sent_at
        note "metadata only, never the body — migration 094; RLS enabled; UNIQUE (kind, entity_id)"
    }
    email_templates {
        text kind PK
        text subject
        text html
        text text_body
        timestamptz updated_at
        text updated_by
        note "admin override; no row means the built-in template — migration 095; RLS enabled"
    }
    email_template_versions {
        bigserial id PK
        text kind
        text subject
        text html
        text text_body
        timestamptz saved_at
        text saved_by
        note "last 20 per kind — migration 095; RLS enabled"
    }
    home_features {
        serial id PK
        text title UK
        text description
        int sort_order
    }
    profile_highlights {
        serial id PK
        text value "e.g. 2M+"
        text label "e.g. transactions a day"
        int sort_order
        bool is_published
        timestamptz created_at "migration 078; RLS enabled; Home hero stat strip"
    }
    courses {
        serial id PK
        text seo_title "migration 085; with meta_description, primary_keyword, og_image (all NOT NULL DEFAULT ''): empty = fallback chain, same four columns on articles and software"
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
        int min_price_cents "the course fixed price, INR paise (0 = free); the only checkout amount source (models.Course.FixedPriceCents); courses sell in INR only, migration 062"
        text content_key
        text content_html "legacy column, left empty"
        text content_object "private Storage path courses/{slug}.html"
        text rag_context "admin-editable passage for the AI chatbot's RAG index; migration 067; see §12"
        text[] tags "same shape as articles.tags; migration 072; powers the /courses topic filter"
        timestamptz created_at
        timestamptz updated_at
        text outline "lesson h2 titles, one per line, derived from the stored lesson; migration 079"
        boolean preview_ready "lesson has a free public preview (storage.LessonPreview); set with the outline; migration 086"
        timestamptz preview_checked_at "last preview-builder attempt; the lazy retry throttle; migration 091"
    }
    slug_redirects {
        text old_path PK "e.g. /courses/old-slug/learn; migration 086, RLS on"
        text new_path
        timestamptz created_at
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
        text module "default empty"
        smallint set_no "nullable, 1-5; attempt N serves set N"
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
        int attempt_no "nullable (admin previews); UNIQUE(user_id, course_id, attempt_no); trigger caps at 5 + grant"
        smallint set_no
        bigint_array question_ids "the stored question set; scoring uses these"
        timestamptz submitted_at "NULL while in progress; set once on grading"
    }
    quiz_attempt_grants {
        bigint user_id PK,FK
        bigint course_id PK,FK
        int extra "0-50, admin-set extra attempts beyond 5"
        timestamptz created_at
        timestamptz updated_at
    }
    purchases {
        serial id PK
        bigint course_id FK
        bigint user_id FK "NULL for a guest purchase until claimed (migration 087); ON DELETE SET NULL"
        text email
        int amount_cents
        text currency
        text status "created | paid | failed"
        text razorpay_order_id UK
        text razorpay_payment_id
        text access_token UK "placeholder scrubbed-<hash>"
        text access_token_hash "SHA-256 of bearer token unlocking /learn"
        bigint promo_code_id FK "nullable"
        text promo_code
        text promo_text "snapshot of the promo code's BannerText/Description at redemption time, for the receipt; migration 063"
        int discount_cents
        timestamptz created_at
        timestamptz paid_at
        text claim_code "unclaimed guest purchases only, upper-case hex, unique where non-empty, wiped when claimed; migration 087"
        timestamptz claimed_at "migration 087"
        note "CHECK (status <> paid OR paid_at IS NOT NULL)"
    }
    software {
        bigserial id PK
        text slug UK
        text title
        text version
        text summary "short one-liner -- catalog card and the product page's hero; migration 077, mirrors courses.summary"
        text description "full write-up -- shown in full on the product page, and inline behind the catalog card's 'Read more' expand"
        text rag_context "admin-editable passage for the AI chatbot's RAG index; see §12"
        text download_link "external URL, e.g. a Google Drive folder -- never a Storage path, there is no file hosted on this site"
        int price_cents "INR, fixed price"
        text license_terms "admin-editable, e.g. 'Licensed per device'; shown on the product page and the receipt"
        text post_purchase_instructions "admin-editable, e.g. how to request a license key by email; shown on the product page and the receipt"
        bool is_published
        int sort_order
        timestamptz created_at
        timestamptz updated_at
        note "migration 075; the fourth paid product, alongside courses/booking/article-contribution"
    }
    licences {
        bigserial id PK
        text lic_code UK "TAB-XXXXX-XXXXX, signed into the key"
        bigint software_id FK
        bigint purchase_id FK "nullable; unique when set: one licence per paid purchase; NULL = added by admin"
        bigint user_id FK "nullable, ON DELETE SET NULL"
        text ver "app major version, e.g. 1"
        text device_hash "normalised hex Device ID; empty until the owner supplies it"
        text key_text "the signed VBL1- key; empty until issued"
        text kid
        timestamptz iat
        timestamptz revoked_at
        text revoked_reason
        text note
        bool added_by_admin
    }
    licence_events {
        bigserial id PK
        bigint licence_id FK
        text event "issued, reissued, device_change, admin_reissue, revoked, added"
        text actor "user or admin"
        text detail
        timestamptz created_at
    }
    software ||--o{ licences : "is licensed via"
    software_purchases ||--o| licences : "creates one"
    licences ||--o{ licence_events : "audit trail"
    software_purchases {
        bigserial id PK
        bigint software_id FK
        bigint user_id FK "nullable, ON DELETE SET NULL"
        text email
        int amount_cents
        text currency
        text status "created | paid | failed"
        text razorpay_order_id
        text razorpay_payment_id
        text access_token "placeholder scrubbed-<hash>"
        text access_token_hash "SHA-256 of bearer token revealing download_link"
        bigint promo_code_id FK "nullable"
        text promo_code
        text promo_text
        int discount_cents
        timestamptz created_at
        timestamptz paid_at
        text claim_code "as purchases; migration 087"
        timestamptz claimed_at "migration 087"
        note "mirrors purchases field-for-field with software_id in place of course_id; migration 075"
    }
    bookings {
        serial id PK
        bigint user_id FK
        text name
        text email
        text phone
        text message
        timestamptz slot_start "partial UNIQUE where status <> cancelled (uq_bookings_active_slot)"
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
        text booking_code "BK-XXXXXX, partial UNIQUE where <> empty; migration 089"
        text manage_token_hash "SHA-256 of the private manage token, partial UNIQUE; the token itself is never stored"
        timestamptz created_at
        note "no-overlap: EXCLUDE USING gist (tstzrange(slot_start, slot_end) WITH &&) WHERE status <> cancelled (bookings_no_overlap, migration 089, skipped with a NOTICE if old rows already overlap). amount_cents/currency/discount_cents/promo_code* are always 0/empty now — hourly consultancy has no payment step; razorpay_order_id is only a unique token for MarkPaid to scope its update to, never a real Razorpay reference"
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
        note "one /contact form submission — no login, no FK, standalone lead capture; a confirmation is queued to the submitted address"
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
        timestamptz updated_at "migration 085; NULL until edited after it; sitemap lastmod = COALESCE(updated_at, created_at)"
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
        text product "course | consultancy | software; migration 075 added software"
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
    promo_code_software {
        bigint promo_code_id PK_FK
        bigint software_id PK_FK
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
        text source_type "profile|skill|experience|education|course|software|article|consultancy|case_study|certification|publication|testimonial; migration 065, software added migration 075"
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
- `purchases.access_token_hash` (SHA-256 of the cookie's token; `access_token` holds a `scrubbed-…` placeholder, same for `software_purchases`; migration 093) is the single mechanism gating `/courses/{slug}/learn` — see `courseAccess()` in `core/handlers/checkout.go`. A `$0` "free" purchase still gets a real row and a real token; it just skips Razorpay entirely (`amountCents == 0` branch in `CheckoutCreateOrder`).
- `audit_requests` is now read-only from the app's perspective — nothing writes new rows anymore since the Architecture Review's self-checkout was removed (see §6). The table and its historical rows are kept purely so the admin payments ledger still shows what was actually paid before the removal.
- `course_updates` has no Go-side fan-out logic — a **Postgres trigger** (`trg_course_updates_notify`, from migration `026_course_updates.sql`) does the INSERT into `admin_messages` for every buyer automatically. Inserting a `course_updates` row (from the admin course-edit form's "what changed" field, or directly via SQL) is the *entire* notification mechanism.
- `min_price_cents` on `courses` defaults to `0` at the column level (migration `030`), but every published course has carried a real, industry-level INR minimum (content, set via the admin course form) since Oct 2026 — the free-by-default "contribute as you like" state from Sept 2026 was retired in favor of real pricing plus whichever promo code is currently active as the free-access path. `CheckoutCreateOrder` (`core/handlers/checkout.go`) now requires a Google-authenticated session unconditionally — there is no guest-checkout branch left.
- **Multi-currency is retired (migration `062`, Oct 2026).** Every price on this site is INR only — `models.CurrencyINR`. There is no more visitor-region detection (`core/geo` and `X-Vercel-IP-Country` are gone entirely), no `courses.currency` or `courses.min_price_cents_inr` column, no `CHECKOUT_CURRENCY` env var, and no `Course.RegionalPrice`/`RegionalPriceDisplay` — `Course.PriceDisplay`/`MinPriceDisplay` render a plain INR figure unconditionally. Article contributions and the consultancy reference figures (`AuditPriceDisplay`, `RetainerRangeDisplay`) are INR-only the same way. Razorpay is still created with `models.CurrencyINR` explicitly passed through, rather than relying on the account's default.
- **Promo codes got three new scoping dimensions in migration `062`, all configurable per code from `/admin/promo-codes`:** a per-account redemption cap (`max_redemptions_per_user`, 0 = unlimited, 1 by default — independent of `max_redemptions`, the old total-across-everyone cap, both enforced in `resolvePromoCode` via `PurchaseRepo.CountUserRedemptions`); a start time (`starts_at`) alongside the existing `expires_at`; and a public/private split — `is_public` codes scroll in the sitewide promo banner (`PromoCodeRepo.ListActivePublic`, fetched concurrently in `startBaseChrome` on every single page load and rendered by the `promo_banner.html` partial in `base.html`, just below the header, on every page), while a code with any `allowed_emails` listed (comma-separated text, parsed in the repo layer, not a join table — it's a short, rarely-edited list) is always private regardless of the `is_public` checkbox: `AdminPromoSave` forces `IsPublic = false` the moment any email is present. `PromoCode.AppliesToAllCourses` is gone — an empty `promo_code_courses` join for a code now means "applies to any course" by itself, with no separate flag that could drift from it.
- Natural-key uniqueness (migrations `050` and `054`): `skill_groups.category`, `experiences (company, role, start_date)`, `education (institution, degree, field)`, `home_features.title`, `availability_rules (weekday, start_time, end_time)`, `quiz_questions (course_id, question)`, `articles.slug` (promoted from the existing unique index), and `UNIQUE NULLS NOT DISTINCT` on `certifications (name, issuer, issued_on)` and `publications (title, venue, published_on)`. Duplicate rows are cleaned in the same migration before the constraint is added, so a second cold start is a no-op.
- RLS with zero policies covers every public table (migration `051`'s lockdown pass, plus every table added since has enabled it directly in its own `CREATE TABLE` migration — e.g. `role_resumes` in `058`, `software`/`software_purchases` in `075`). The app connects as the table owner and bypasses RLS; this only closes the PostgREST `anon`/`authenticated` door. **A table created without it is a live security gap, not just lint noise** — `chatbot_reindex_pending` (migration `070`) was missed this way and briefly sat exposed to PostgREST with no RLS at all until Supabase's security advisor flagged it and migration `076` fixed it. Check the security advisor (or grep for `ENABLE ROW LEVEL SECURITY`) after any new `CREATE TABLE` migration.

---

## 4. `core/handlers` — the App struct and request flow

`handlers.App` (in `core/handlers/app.go`) is the one struct every handler is a method on. It holds:
- `Cfg *config.Config`, `DB *sql.DB`, `View *view.Renderer`, `Auth *middleware.Auth`
- One repo field per aggregate: `Profile, Skills, Experience, Resumes, Education, Features, Courses, Bookings, Availability, Admins, Purchases, Users, Articles, PromoCodes, QuizQuestions, QuizAttempts, QuizGrants, AdminMessages, CourseUpdates, Testimonials, Certifications, Publications, CaseStudies, AuditRequests, ArticleVotes, ArticleContributions, ContactMessages, Subscribers, CourseReviews, ChatbotChunks, ChatbotRateLimits`, plus `CourseStorage` for the private buckets
- `Razorpay *payments.Razorpay`, `Telegram *notify.Telegram`, `Google *oauth.Google`, `Nvidia *ai.Client`, `Timezone *time.Location`

Shared page chrome is assembled by `base` / `startBase` / `baseFrom` in `app.go`. `BaseData` carries site name, title, meta description, canonical URL, nav links, login/admin state, the footer's LinkedIn link, notifications, and `AssetVersion`. Every page-specific `*Data` struct embeds it.

Chrome lookups (the profile row, for the LinkedIn link; plus, for a signed-in visitor, user name and notifications) stay on their own `sync.WaitGroup` on purpose: that contract is "degrade quietly" — a missing LinkedIn link or an empty notification menu is a fine outcome for a page that isn't about either. Page-owned lookups use `errgroup` and fail the render. Folding chrome into the errgroup would silently promote a swallowed error into a 500; the inverse would swallow a missing About section.

Those chrome lookups used to run as a **second sequential wave** after each page's own `errgroup.Wait()`, and pages that already loaded `profile` fetched it again. On the public hot path the handler now calls `startBase` (or `startBaseWithProfile` when it already loads that row) *before* its own lookups and finishes with `baseFrom` after them, so chrome rides in the same round-trip wave. `base()` remains the one-shot form for admin pages, error pages, and form re-renders that have no wave of their own to join.

Anonymous `/about` is therefore one wave of eight errgroup lookups. Anonymous `/` is one wave of four. `/articles` is one wave of the filtered page (count + rows, themselves concurrent under an errgroup inside `ArticleRepo.ListPublishedFiltered`), the topic list, and chrome. List queries select `articleListColumns` — everything except `body`, which only the article's own page needs. `/courses` is the same three-way shape as `/articles` — `CourseRepo.ListPublishedFiltered` (filtered/sorted/paginated page, its own count+rows concurrent), `ListAllTags`, and the full unfiltered catalog that backs the always-complete "Where to start" tracks — plus chrome, all in one wave. `ListPublishedFiltered`'s query carries two `LEFT JOIN` aggregate subqueries, neither backed by a stored column on `courses`: `review_count` (from `course_reviews`, the only review signal ever shown publicly — never the rating value, see migration 064) and `enrolled_count` (`1 + COUNT(purchases WHERE status = 'paid')` per course — the `+1` is a display floor so a brand-new course never reads "0 enrolled"). Both are computed live on every request rather than maintained as incrementing counters, so neither can drift from what `course_reviews`/`purchases` actually contain, and `checkout.go` needed no changes to make an existing or new purchase count — it already does, automatically, the next time the query runs. `/resume.pdf` has no chrome. A role that has its own file is one lookup plus the Storage read. Every other request loads the default row (`role_key` `default`) and reads that object. With no default file stored it is one wave of four and generates a PDF. The Person schema's `knowsAbout` is derived from the skill groups the home page has already loaded, not a separately maintained keyword list.

`App.Routes()` (in `routes.go`) builds the full `http.Handler`: one public `ServeMux`, one admin-auth `ServeMux`, one `RequireAdmin`-wrapped protected `ServeMux` for the rest of `/admin/`, composed and wrapped in `middleware.Logging(middleware.SecurityHeaders(middleware.CanonicalHost(siteURL)(middleware.DomainRouter(domain)(mux))))`. Public utility and gated routes that can render HTML but are not Google landing pages (`/search`, `/login`, `/booking/manage/{token}`, `/account`, admin, and gated course learn/quiz/certificate pages) are wrapped with the shared `noindex` helper rather than added to the sitemap. The innermost layer around the mux (inside `Auth.Attach` and `redirectAccessToken`) is `App.errorPages` (`error_pages.go`): it swaps a plain-text 500 for the `server_error` page (the writer wrapper discards the plain body; nothing in the app uses Flusher or Hijacker) and recovers panics (re-raising `http.ErrAbortHandler`), logging and alerting to Telegram. It skips `/internal/` and `/api/`. `a.serverError` also alerts. `api/index.go` answers 503 with `Retry-After` and a generic message when `Bootstrap` fails, logging the real error. See `docs/FUNCTIONALITY.md` for what each route does; the exact route table (method + path + handler) is the source of truth in `routes.go` itself — don't let this doc's prose drift from it, read `routes.go` directly for the literal list.

Handler files, by area:
| File | Covers |
|---|---|
| `public.go` | Home (incl. subscribe-form data), About (incl. case studies), Resume, CoursesList (incl. subscribe-form data), CourseDetail, Sitemap-adjacent data |
| `checkout.go` | Course purchase order/verify (fixed price, guest or signed in), `courseAccess()` gate |
| `purchase_common.go` | `resolveBuyer` (account or email-only guest; courses), `finalAmountCents`, course success page (`/courses/{slug}/enrolled`), `/account/claim` |
| `course_contribute.go` | Optional post-course contribution order/verify, promo quote (`CheckoutQuote`) |
| `software.go` | Public Software list/detail |
| `software_checkout.go` | Software purchase order/verify (fixed price), `softwareAccess()` gate |
| `admin_software.go` | Admin CRUD for Software |
| `consultancy.go` | Consultancy page (tiers, FAQ, case studies, JSON-LD), public Booking page/confirm (no login, free, no payment gateway; honeypot + `auth_throttle` kind `booking`), Contact page/submit |
| `admin_contact.go` | Admin contact-messages list + mark-read |
| `admin_credentials.go` | Admin CRUD for Certifications and Speaking & Publications |
| `admin_case_studies.go` | Admin CRUD for Case Studies |
| `newsletter.go` | Public `POST /subscribe` (records the address and sends the confirmation when Brevo is configured) + admin subscribers list/delete |
| `email_send.go` | Enqueue after commit, worker send, `GET /internal/email/retry` |
| `email_build.go` | Builds one job per purchase, booking, contribution, consultancy connect request, and newsletter confirmation |
| `admin_email_templates.go` | Admin template list, edit, preview, send-test, reset, restore, and queue resend |
| `admin_inbox.go` | Admin inbox: Resend Inboxes API (list, view, reply, forward, labels, threads, drafts, agent, attachments). Bodies stay at Resend |
| `articles.go` | Public article list (incl. subscribe-form data)/detail/thumbs-up + Admin article CRUD |
| `account.go` | `/account` page + PDF receipts + testimonial submission |
| `booking_reschedule.go` | Admin and signed-in booking reschedule flows, `availableDays` (30 days) and `bookableSlotError`, the single server-side gate for any requested slot |
| `booking_manage.go` | Token-based manage page for no-login bookings: view, cancel, reschedule (`manageGuard` sets noindex, no-store, no-referrer) |
| `learn.go` | Gated course content serving inside the site chrome + watermark injection |
| `quiz.go` | Quiz intro/start/resume (5-attempt engine), scoring, certificate download, public certificate verification (`/certificate/verify/{code}`) |
| `course_reviews.go` | Verified-buyer review submit (private rating/comment) + its admin-only list view |
| `auth.go` | Login page, Google OAuth start/callback, logout |
| `legal.go` | Generic static legal-page handler factory |
| `seo.go` | `baseURL`/`pageURL` helpers, canonical rule (`canonicalURLForRequest`), Sitemap (`renderSitemap`, `staticLastmod`), Robots, LLMsTxt, IndexNow key + submit, Search |
| `seo_fields.go` | SEO plumbing: `ogImageFor`/`absoluteURL`, `markNoindex`/`robotsMetaForRequest`, `pageSEOApplies`, `seoFieldsAsync`, breadcrumb `trail` |
| `admin_seo.go` | `/admin/seo` page (one form per static page, `page_seo` upsert/reset) and the shared `seoFieldsFromForm`/`saveSEOFields` used by the course, article and software forms |
| `schema.go` | JSON-LD builders, all fed from DB rows (Person/Website/Service on Home; ProfessionalService + FAQPage on Consultancy; BlogPosting; Course + Offer; SoftwareApplication + Offer; BreadcrumbList) |
| `admin.go` | Admin login, dashboard, course CRUD, bookings, purchases, availability, profile, preview-as-visitor |
| `media.go` | Image-upload validation (`readImageUpload`, `sniffImageExt`) shared by the profile photo and logo; public `GET /media/profile-photo` and `GET /media/logo` streaming handlers |
| `admin_content.go` | Admin CRUD for skills, experience, education, testimonials |
| `admin_users.go` | Admin user list/detail/promote/demote/message |
| `admin_quiz.go` | Admin CRUD for quiz questions + list page |
| `admin_quiz_bank.go` | Quiz bulk JSON import, auto-assign sets, per-user attempt grants |
| `admin_promo.go` | Admin CRUD for promo codes |
| `admin_payments.go` | The combined payments ledger (purchases + bookings + audit requests + article contributions + software purchases merged) |
| `promo.go` | `resolvePromoCode()` — the one shared discount-resolution function every checkout flow calls |
| `chatbot.go` | Public `POST /chat` (RAG answer) + `GET /ai-assistant` (`AIAssistantPage`) + `chatbotSessionID`; admin `/admin/chatbot` page + `/admin/chatbot/reindex` + content-gathering/chunking helpers — see §12 |

**Transactional email** (`core/notify`, `core/handlers/email_send.go`, `email_build.go`). Mail is queued for every completed order, including a zero amount and a promo, every booking, including a free booking, and every consultancy connect request (`POST /contact`, the form the consultancy page links to). The customer request writes one `email_outbox` row after the purchase, booking, contribution, contact message, or subscriber row is committed, then sends it before returning. It does not start a goroutine. The Go runtime on Vercel freezes when the handler returns, so a detached send would never run. The send times out after 4 seconds. A provider error marks the row failed and does not fail the caller. `email_outbox` records that send. `(kind, entity_id)` is unique. `entity_id` is the course purchase id, software purchase id, article contribution id, course contribution id, booking id, contact message id, or subscriber id. A second verify adds nothing. A second reschedule of the same booking puts that one row back to `pending` and sends it again. The rendered body is never stored. The connect confirmation does not include the message they typed. The private booking manage token is not stored either, so the emailed link is `/account` or `/booking`; the on-screen link remains the only copy of the token. A queue problem does not undo the order, the booking, or the connect request. Password reset stays the security question.

`GET /internal/email/retry` is the daily fallback, not the first send. Vercel Cron calls it once a day (`0 3 * * *` in `vercel.json`). A more frequent schedule is what Hobby rejects, and that rejection blocks the deployment from being created. It sends `pending` rows and retries `failed` rows with fewer than five attempts. Send-test rows are excluded. The bearer is `CHATBOT_REINDEX_TOKEN` (constant-time compare, 404 when unset). Vercel sends `Authorization: Bearer` from `CRON_SECRET`, so that variable must be the same value as `CHATBOT_REINDEX_TOKEN`. There is no GitHub Actions workflow for email. The chatbot poller stays on GitHub Actions because it needs a few minutes, which Hobby cron cannot do. Public forms that cause an email to an arbitrary address are protected the same way as booking: honeypot field `website`, plus `auth_throttle` counters keyed by the hashed client IP (`throttleBooking`, `throttleContact`, `throttleSubscribe`), and `buyerName` refuses to echo spam-looking names into the message. After a successful send, `sendQueued` posts a Telegram copy through `emailTelegramCopy` (`email_send.go`): headers and plain-text body, with `AccessURL`, `ManageURL`, `ClaimCode` and `UnsubscribeURL` replaced by a note and the text clipped to 3,900 runes. An empty provider key leaves the row `pending`. A provider error marks it `failed`, raises a Telegram alert, and does not fail the original order, booking, or connect request. Purchases, payments, and bookings in the admin console show sent, failed, pending, or none. Admin resend is offered for pending, failed, and disabled rows. It sets the row back to `pending` and sends it in that request. An empty provider key leaves the row pending. The template send-test sends immediately, and only to `ADMIN_BOOTSTRAP_EMAIL`.

`RESEND_API_KEY` and `BREVO_API_KEY` are the only new mail environment variables. A provider sends only when its key is non-empty. From addresses are constants in `core/notify/mail.go`: Vishnu Balachandran `<orders@vishnubalachandran.com>` for course, software, article, and contribution mail, including a zero amount, `<bookings@vishnubalachandran.com>` for bookings and consultancy connect requests, `<news@vishnubalachandran.com>` for the newsletter confirmation. Reply-To for course, software, article, and contribution mail is `orders@vishnubalachandran.com`. Reply-To for bookings, consultancy connect requests, and the newsletter confirmation is `admin@vishnubalachandran.com`. BCC and the admin send-test recipient are `ADMIN_BOOTSTRAP_EMAIL`. News stays on the main domain; a later promo subdomain is a one-line change to the news From constant. Keys are never written into the repo, docs, or logs.

The admin inbox (`/admin/inbox`, `core/handlers/admin_inbox.go`, `core/notify/resendinbox.go`) reads and replies through the Resend Inboxes API with the same `RESEND_API_KEY`. It does not add an environment variable and it does not store message bodies in Postgres. Mail shows up only after the domain's receiving is pointed at Resend. Receiving DNS is the owner's; the app does not change it. An empty key leaves the inbox pages explaining that Resend is off.

---

## 5. Auth — two independent JWT sessions, password and Google sign-in

Both live in `core/middleware/auth.go`, sharing one `Auth` struct (one JWT secret, one TTL — 7 days) but **separate cookies and separate context keys**:

| | Admin | User (visitor) |
|---|---|---|
| Cookie name | `portfolio_admin_session` | `portfolio_session` |
| Claims | `{admin_id}` | `{user_id}` |
| Gate middleware | `Auth.RequireAdmin` | `Auth.RequireUser` |
| Unauthenticated redirect | `/admin/login` | `/login?next=<path>` |
| Context accessor | `middleware.AdminIDFromContext` | `middleware.UserIDFromContext` |
| Non-redirecting check | `Auth.IsAuthenticated(r)` | `Auth.IsUserAuthenticated(r)` |

**Visitor login is a password or Google** (`core/handlers/auth.go`, `auth_password.go`). `GET/POST /login`, `/signup`, `/forgot`, `/forgot/reset` and `/account/security` handle password accounts (bcrypt, 10 to 72 characters, `authutil.ValidatePassword`); the Google button (`core/oauth/google.go`, a standard authorization-code flow with a CSRF `state` cookie) stays alongside. **Signup does not verify the address** (transactional mail is a receipt or a booking message, not proof of the inbox). An email typed at signup is never treated as verified. Consequences: (1) password recovery is the **security question** chosen at signup, from a fixed list (`securityQuestions`); the answer is trimmed, lower-cased and stored only as a bcrypt hash; (2) an unknown, Google-only, or admin email is shown a stable, believable fake question (HMAC of the email under `JWT_SECRET`) so the page does not reveal who has an account, and the answer is compared against `authutil.DummyHash` so timing matches; (3) when Google later proves an email that has an unverified password account, `UserRepo.MarkGoogleVerified` clears that password and answer and bumps `token_version` (the real owner takes over); (4) a password login never issues an admin session; (5) `GET/POST /account/delete` (`AccountDeletePage/Submit`) calls `UserRepo.DeleteAccount`, one transaction that anonymises bookings, purchases, software_purchases, article/course contributions and quiz_attempts, deletes testimonials and thumbs-up, then the user row (other FKs are SET NULL/CASCADE); it refuses the admin and users with an upcoming booking. Breach checklist: `docs/SECURITY.md`.

**Sessions stay stateless** (a signed HS256 JWT cookie, HttpOnly, Secure in production, SameSite=Lax, 7-day expiry; the parser accepts HS256 only). The user cookie carries `tv`, the user's `token_version`; `Auth.SetVerifiers` (wired in `handlers.New`) re-checks every cookie against Postgres, so a password reset or change (`UserRepo.SetPassword` bumps the version) signs every old session out, and an admin removed from `admins` loses access immediately instead of at cookie expiry. `Auth.Attach` memoises that lookup once per request (an anonymous request costs no query). `middleware.SameOrigin` blocks cross-site POSTs (CSRF) for the whole mux; requests with no `Origin` header, such as the token-guarded reindex call, pass.

**Throttling** is in Postgres (`auth_throttle`, `repo.AuthThrottleRepo`) because serverless instances share no memory: five failures per email or per client IP lock that key for 1 minute, doubling per further failure up to 60 minutes; the window resets after an hour of quiet. Kinds: `login`, `reset`, `signup` (10 per IP per hour, honeypot field too), `admin` and `booking` (public booking: 6 per IP, 3 per email, honeypot too). Keys are SHA-256 digests, never raw emails or IPs.

Admin login (`/admin/login`) takes an admin email plus the password and compares the password, in constant time (`authutil.SecretEquals`), with the `ADMIN_BOOTSTRAP_PASSWORD` environment secret. **No admin password or hash is stored in the database** (migration 080 dropped `admins.password_hash`); the `admins` table is only the list of admin emails, and one password covers all of them. `ADMIN_BOOTSTRAP_EMAIL` is added to that list on every start (`bootstrapAdmin` in `core/app/bootstrap.go`) and cannot be demoted from the console. Changing the variable and redeploying changes the password. An unset variable disables password login (Google sign-in as an admin email still works). Any user can be promoted to admin from `/admin/users`.

**Admin preview-as-visitor** is a third, lightweight state: a plain cookie flag (`preview_visitor`), not a session — `isPreviewingAsVisitor(r)` short-circuits `base()`'s `isAdmin` computation to `false` while the admin's real session cookie stays intact, so `/exit-preview` just clears the flag.

---

## 6. Payments — Razorpay powers two live paid flows: courses and software

**Course purchases and software purchases are the two paid flows left on this site.** Consultancy (Architecture Review, retainer, hourly) never touches a payment gateway at all — no code path, no dormant endpoint, nothing to re-enable by flipping a flag. This was a deliberate removal, not a disable: the site owner isn't taking payment integration further for consultancy, and having a live-but-unlinked paid endpoint sitting in the background was considered worse than just deleting it. If self-checkout for consultancy is ever wanted again, it needs to be rebuilt — `git log` on `consultancy.go`/`booking.html` around the removal commit is the reference for what the Razorpay version looked like.

**Course purchase** (`POST /courses/{slug}/checkout/order` → `.../verify`, `purchases` table) follows: **create-order handler** (server resolves the true price, creates a Razorpay order, writes a `pending` DB row) → client opens Razorpay's Checkout.js modal with that order ID → **verify handler** (HMAC-verifies the callback server-side via `Razorpay.VerifySignature`, flips the DB row to paid, fires a Telegram notification, returns a redirect). Access is **only ever granted from the verify step**, never trusted from the client-side "success" callback alone. It **skips Razorpay entirely when the resolved amount is $0** — see §3. Pricing here is pay-what-you-want, floored at `courses.min_price_cents`.

**Software purchase** (`POST /software/{slug}/checkout/order` → `.../verify`, `software_purchases` table, `core/handlers/software_checkout.go`) follows the *exact same* create/verify/HMAC/cookie/$0-skip shape as course purchase, copied field-for-field rather than branched into `checkout.go` — the one real difference is pricing: a software item sells at one fixed price (`software.price_cents`), set and edited only in the admin console, never supplied by the client. A verified purchase doesn't unlock in-browser content the way a course does — it reveals `software.download_link` (an external URL, e.g. a Google Drive folder) plus the admin-editable `license_terms`/`post_purchase_instructions` free-text fields, on the software's own detail page and in the buyer's `/account` "My Software" section and PDF receipt.

**Consultancy hourly** (`POST /booking/confirm`, `bookings` table) is public (no login) and is a plain "reserve the slot, confirm it" flow with no price and no payment step of any kind. `BookingConfirm` validates, throttles, then calls `BookingRepo.CreatePublic`, which inserts the row already `confirmed` with a `BK-XXXXXX` code and the SHA-256 of a one-time manage token. The slot race is decided by the database: the partial unique index `uq_bookings_active_slot` plus the `bookings_no_overlap` exclusion constraint (migration `089`) reject a second live booking for the same or an overlapping time (`isUniqueViolation` maps SQLSTATE 23505 and 23P01 to `ErrSlotTaken`). `CreatePending`/`MarkPaid` remain in the repo but nothing calls them. The token is returned to the browser once and never stored in clear; `/booking/manage/{token}` looks the booking up by hash (`booking_manage.go`). Throttling uses `auth_throttle` kind `booking`: 6 attempts per hashed IP and 3 per hashed email before the growing lock; an attempt counts once it passes validation, success or not.

**Architecture Review and Fractional Advisory** have no create/verify endpoints at all — both route to `/contact` from `consultancy.html`, to be scoped and invoiced outside this site. The `audit_requests` table and `AuditRequestRepo.ListAll` still exist, read-only, purely so the admin payments ledger (`admin_payments.go`) can keep showing the real historical Architecture Review purchases from when self-checkout briefly existed; nothing writes to that table anymore. **Do not copy `audit_requests.go`'s shape for a new paid product — it is dead code, kept only for that historical ledger.**

`core/payments/razorpay.go`: `Razorpay.Enabled()` is `false` whenever `RAZORPAY_KEY_ID`/`RAZORPAY_KEY_SECRET` aren't set — both create-order handlers check this (except each one's own $0 branch, which doesn't need Razorpay at all) and return `503` with a friendly message instead of erroring.

**Promo codes are product-scoped** (`promo_codes.product`, a `CHECK` of `'course' | 'consultancy' | 'software'`). `resolvePromoCode()` (`core/handlers/promo.go`) is one shared function, not three near-duplicates — it branches internally only where the products genuinely differ: per-item scoping applies to the course product (`promo_code_courses`) and the software product (`promo_code_software`, migration `090`; an empty list means any item for either), the `itemID` argument being the course or software ID; `PromoCodeRepo.loadScope` fills both lists and `insertPromoScope` writes them, and per-user redemption counting branches between `PurchaseRepo.CountUserRedemptions` (courses) and `SoftwarePurchaseRepo.CountUserRedemptions` (software), since paid purchases of each product live in their own table.

**To add a new fixed-price paid product**, `checkout.go`'s `CheckoutCreateOrder`/`CheckoutVerify` + `purchases` table (fixed price, guest or signed in) and `software_checkout.go`'s `SoftwareCheckoutOrder`/`SoftwareCheckoutVerify` + `software_purchases` table (fixed price) are the two live reference implementations for a Razorpay create/verify pair in this codebase — pick whichever pricing model fits and copy that one's shape into its own new file(s), per the layering rule in §2 above.

---

**Guest checkout and claim codes (migration `087`).** `purchases` and `software_purchases` gain `claim_code TEXT NOT NULL DEFAULT ''` (unique where non-empty) and `claimed_at`. A purchase made without an account has `user_id` NULL and a random claim code (`authutil.NewClaimCode`: 20 upper-case hex characters, stored without separators). Claim codes are issued for **course** purchases only (software checkout stays login-required; its `claim_code` columns exist for symmetry and stay empty). `core/repo/claims.go` (`ClaimRepo`) attaches purchases atomically: `ClaimByCode` (used by `/account/claim`, throttled with `auth_throttle` kind `claim`) and `AttachByEmail` (called only from the Google callback after Google has verified the email; a typed or password-signup email must never reach it). Both update only rows still having `user_id IS NULL` and a non-empty code, wipe the code in the same statement, and move the matching `licences` row (software) in the same transaction. Course prices are fixed: `models.Course.FixedPriceCents()` (column `min_price_cents`) is the only amount source; no amount is read from the request. Guest checkout starts are throttled per IP in `auth_throttle` kind `checkout`. `POST /courses/{slug}/checkout/quote` previews the promo summary via `resolvePromoCode`. The optional post-course contribution (`course_contribute.go`, table `course_contributions`, repo `CourseContributionRepo`, migration 087) is a third Razorpay create/verify pair modelled on `article_checkout.go`: access-gated by `courseAccess`, amount range-checked server-side, paid only after signature verification.

## 7. Templates / rendering (`core/view`)

`view.New()` parses every page template **twice-composed**: once against the public layout (`base.html` + `header.html` + `footer.html` + `skills_grid.html`) for public pages, once against the admin layout (`admin_base.html` + `adminnav.html`) for admin pages — two explicit lists in `view.go`, so **a new page template must be added to one of those two lists** or `Renderer.Render` will 500 with "template not found". `Renderer.Render(w, status, "page_name", data)` executes the matched layout's entrypoint template, passing `data` as `.`.

`publicLayout` also carries **shared component partials** — templates more than one page renders, kept in one place rather than copy-pasted. `promo_banner.html` (`{{define "promo_banner"}}`, reads `.PromoCodes` off whatever `BaseData` it's handed) is called exactly once, from `base.html` itself right after the header — so it renders on *every* public page, not per-page — and lists every currently active, public promo code (`PromoCodeRepo.ListActivePublic`, fetched concurrently in `startBaseChrome`); empty renders nothing. A pure-CSS marquee (`.promo-banner-track`, `@keyframes promo-banner-scroll` in `style.css`), disabled under `prefers-reduced-motion`. `skills_grid.html` (`{{define "skills_grid"}}`, taking a `[]models.SkillGroup` as its dot) is the first: Home's "My story" and About's "Skills" both call `{{template "skills_grid" .Skills}}`, which is what stops the two grids from drifting apart the way two copies of the same markup did before. Add a new shared component to that list too. Its interaction is CSS-first — see the `.skills-grid` comment block in `web/static/css/style.css`. There are **two display modes with different layout mechanics**, which is the one non-obvious thing about this component: collapsed, the card face shows category, level, and four chips (plus "and N more") and a revealed panel is `position: absolute` over its own card (achievement summary plus the full tag list), and being out of normal flow is exactly what stops the grid reflowing and keeps the summary out of the resting layout; expanded ("Expand all"), the `.is-expanded-all` modifier returns every panel to `position: static` so the cards size to their content and CSS Grid equalises each row. The second mode exists because the first cannot scale to nine simultaneous panels — nine absolutely-positioned boxes overlap their neighbours and overflow the grid. `main.js` owns the mode toggle, the per-card click/tap toggle (which it suppresses while in expanded mode, so `aria-expanded` never disagrees with what is on screen), and the live-region announcement.

**Page-to-page navigation stays a real, server-rendered multi-page app — no client-side router, no fetch-and-swap.** Clicking an internal link in `main.js` never calls `preventDefault()`; it only schedules `#page-loader`/`#page-loader-mark` (a route-progress bar + a small branded badge, `base.html`) to fade in ~60ms after the click, cleared on the new page's own `pageshow` — a purely cosmetic bridge over the normal browser navigation's network/server wait, not a background data fetch. `style.css` additionally opts into the browser-native **cross-document View Transitions API** (`@view-transition { navigation: auto; }`) — zero JS, zero framework, pure progressive enhancement (unsupported browsers, e.g. Firefox, just navigate with no transition, no broken behavior either way) — which cross-fades the outgoing/incoming page once the new document has already arrived and parsed. The two are complementary, not redundant: the loader covers the wait *before* the new page is ready; the view transition smooths the visual handoff *once* it is. Neither has any SEO bearing — URLs, status codes, meta/OG/JSON-LD, and crawlability are all unchanged; this is compositor-level visual polish applied after the fact, not a change to what gets served or when.

Custom template functions (`view.go`'s `funcs` map): `currentYear`, `formatDate`/`formatDateOptional`/`formatTime`/`formatDateTime`, `money`, `add1`, `splitLines` (blank-line paragraph splitting), `articleBody` (the HTML-or-plain-text blog body renderer — HTML only if the trimmed content *starts* with `<`, otherwise falls through to escaped `splitLines`-style paragraphs; see the doc comment in `view.go` for why prefix-only detection matters), and `rich` (`**bold**`/`*italic*` only — HTML-escapes the input first, then turns just those two marker patterns into `<strong>`/`<em>`, so there's no raw-HTML path to sanitize against; used on admin-authored free-text fields that aren't a full HTML document the way an uploaded article/lesson body is — profile tagline/hero subtitle/summary, skill achievements, experience bullets, education details, testimonial quotes, case study fields, publication summaries, course summary/description, and article summaries).

Every page's `*Data` struct embeds `handlers.BaseData` (site name, title, meta description, canonical URL, nav links, login/admin state, notifications, `AssetVersion`). A page that needs JSON-LD defines a `{{define "jsonld"}}` block that `base.html` calls via `{{block "jsonld" .}}{{end}}`.

---

## 7b. SEO in the database (migration 085)

**No SEO copy lives in code or in the repo.** Two stores, both edited in the admin console:

- `page_seo` (RLS enabled) holds per-path overrides for the static pages listed in `models.StaticSEOPages`. A page with no row renders its built-in title/description unchanged. `startBaseChrome` looks the row up concurrently with the rest of the chrome, only when `pageSEOApplies(r)` (a static path, no `tag`/`q`/`page>1`), so no other route pays for the query. `baseFrom` then applies title, description, OG image and `noindex` (rendered as `<meta name="robots" content="noindex, follow">`).
- `courses`, `articles` and `software` each carry `seo_title`, `meta_description`, `primary_keyword`, `og_image` (all `NOT NULL DEFAULT ''`). They are read and written only through `repo.SEORepo` (`GetFields`, `GetFieldsBySlug`, `SetFields`), deliberately **not** through `CourseRepo`/`ArticleRepo`/`SoftwareRepo`, so the catalog column lists and scans are unchanged. Detail handlers start `seoFieldsAsync` alongside their own queries (best-effort: a failure just means the fallback chain).

**Fallback chain** (`models.SEOFields`, unit-tested in `core/models/seo_test.go`), so new content needs no code change: title = `seo_title` then the existing `"<title> — <site>"` pattern; description = `meta_description` then `summary` (markdown markers stripped) then `description` truncated to 160 at a word boundary; image = `og_image` then the item's cover image then `/static/img/og-image.png`. The 60/160 limits are advisory, shown as live counters (`data-count-max`, `admin.js`), never enforced.

**JSON-LD** is generated by `schema.go` from the same resolved values: `courseJSONLD` (Course with INR Offer from `min_price_cents`, tags as keywords, level, online instance), `articleJSONLD` (BlogPosting), `softwareApplicationJSONLD`, `personAndWebsiteJSONLD` (Person with a stable `#person` `@id`; the Service block and `knowsAbout` come from the profile and skill items, never literal copy), `consultancyJSONLD`, and `breadcrumbJSONLD` (assigned to `BaseData.BreadcrumbJSONLD` by the listing and detail handlers, emitted by `base.html`).

**Technical rules:** canonical drops `tag`, `sort`, `q`; keeps only `page>1` (`canonicalURLForRequest`). 404 renders set `BaseData.NoCanonical` (no `rel=canonical`, no `og:url`). `noindex(...)` in `routes.go` now also marks the request context so `baseFrom` emits the robots meta tag next to the `X-Robots-Tag` header. `GET /courses?page=` 301-redirects (all courses render on one page, `coursesPageSize`). `middleware.LegacyRedirects` (innermost wrapper in `Routes()`) 308-redirects trailing slashes and 301-redirects `/blog[/...]` to `/articles[/...]`, `/course/...` to `/courses/...`, `/book` to `/booking`; `/admin` and `/static` are exempt. The sitemap carries no `priority`, omits `/ai-assistant` and `/resume.pdf`, drops any page whose `page_seo.noindex` is set, and writes `lastmod` only when known (`staticLastmod`: newest of the page's `page_seo` edit and the content it lists; article lastmod is `COALESCE(articles.updated_at, created_at)`, set by `ArticleRepo.Update`; course/software use `updated_at`, which `UpdateOutline` deliberately does not bump).

## 8. Domain routing (`core/middleware/domain.go`, `canonical.go`)

The public canonical host is `www` (`SITE_URL`, currently `https://www.vishnubalachandran.com`). `CanonicalHost` 301-redirects the bare apex and the old `home.` landing host to that www origin with the same path; the www host itself is **never** redirected (a canonical URL that 3xxs is Google's "Page with redirect" indexing failure). Preview hosts (`*.vercel.app`) and local dev are served as-is. `vercel.json` repeats the apex/`home.` → www 301 at the edge so HTTP apex is not a two-step chain (HTTP→HTTPS apex, then apex→www).

Optional (`DOMAIN` env var) subdomain-per-page-group scheme: `about.`, `courses.`, `consultancy.`/`booking.`, `articles.`, `admin.` each own their path prefix on their own subdomain. Unmatched paths (home, search, legal, static) live on **www**, not `home.`. A request on www is always served — including page-group paths — so the live path-based site stays indexable. Apex and `home.` 301 to www; other page-group subdomains still 301 to the subdomain that owns the path. An unrecognized host is served as-is. `SubdomainForPath(path)` is the same lookup, exposed for building absolute URLs; `"home"` resolves to the www host in `pageURL`. With `DOMAIN` unset, `DomainRouter` is a no-op and `CanonicalHost` still consolidates apex/`home.` onto www whenever `SITE_URL` prefers www.

**Incident: filtered/paginated list pages all declared canonical to the bare list page, which Search Console correctly flagged as "Duplicate, Google chose different canonical than user" (Oct 2026).** `BaseData.CanonicalURL` used to be computed as `base + r.URL.Path` only — every query-string variant of `/courses` and `/articles` (a tag filter, a later page, a search) declared the exact same canonical as the plain unfiltered list. That's wrong for two of those four query parameters: `/courses?tag=java` and `/articles?tag=Kafka` are real, internally-linked pages (the topic nav renders them as ordinary `<a href>`s) showing a genuinely different set of items than the bare list — declaring them canonical to a page with different content is a lie Google is entitled to disbelieve, and confirmed live that it does, which is exactly what that Search Console report means. The fix, `canonicalURLForRequest` (`seo.go`, used by `baseFrom` in `app.go`): keep `tag` and `page` (when `page > 1`) in the canonical — each is a genuinely different, independently crawl-worthy set of items, matching Google's own current pagination guidance (self-referential canonical per page, not everything collapsed onto page 1) — but drop `sort` (it only reorders the identical item set, so declaring the sorted view canonical to the unsorted one is *correct*, not a bug) and `q` (a site-internal search result isn't content worth a canonical identity *at all* — see below). `ArticlesPage`/`CoursesList` also now pass the active `tag`/`page` through `catalogListTitle` (`articles.go`) so a filtered page's `<title>` is genuinely distinct too, reinforcing the self-referential canonical instead of contradicting it with an identical title across every variant. Both handlers additionally set `X-Robots-Tag: noindex, follow` whenever `?q=` is present — Google's long-standing guidance is that internal search-results pages shouldn't be indexed at all (thin, endlessly-variable, duplicate-with-everything-else content), so a search hit gets excluded outright rather than left to fight over a canonical. Covered by `TestCanonicalURLForRequest`/`TestCatalogListTitle` in `core/handlers/seo_canonical_test.go` — any future change to `/courses`'/`/articles`' query-param vocabulary should extend those cases, not just trust manual testing.

---

## 9. Deployment

- **Vercel**, one Go serverless function. `api/index.go`'s `Handler` calls `app.Bootstrap()` exactly once per cold start (`sync.Once`) and reuses the resulting `*handlers.App` (including its DB pool) across warm invocations — never re-bootstrapped per-request.
- **`app.Bootstrap()`** (`core/app/bootstrap.go`): loads `config.Load()` (fails fast if `DATABASE_URL`/`JWT_SECRET` are unset), opens the Postgres pool, **runs pending migrations** (`db.Migrate` — tracked in a `schema_migrations` table by filename, so it's safe to run on every cold start; only un-applied files execute), builds the `view.Renderer`, constructs `handlers.App`, and — if `ADMIN_BOOTSTRAP_EMAIL` is set — adds it to the `admins` list (a no-op when it is already there). `ADMIN_BOOTSTRAP_PASSWORD` is read at login only and never written to the database.
- **Static assets and templates are compiled into the binary** via `go:embed` (`web/embed.go` for CSS/JS/images, `core/view/view.go`'s `templateFS` for HTML) — there is no filesystem read at runtime for any of this, which is what makes a single stateless serverless function workable. Paid lesson HTML is not in the binary. `core/storage` reads it from the private `course-content` bucket after `courseAccess` passes (`SUPABASE_SECRET_KEY`, or the anon key plus the dedicated storage-reader login) and `PrepareLesson` returns the iframe document (`PreparedLesson.Document`), which the gated route `GET /courses/{slug}/learn/lesson` serves: the lesson body, the lesson's own `<style>` (copied out of `<head>` so labs, compilers, and animations still run), the site stylesheet, a small reset so the site-wide `ul { list-style: none }` and any collapsed `word-spacing` cannot flatten lesson prose, and the watermark. `/courses/{slug}/learn` renders the site chrome and the reader loads that document into its `.html-renderer` iframe (`src`, no longer an inline `srcdoc`) (`sandbox="allow-scripts"`, no `allow-same-origin`) so the parent cannot read the lesson DOM. The watermark paints only inside that pane. The same login reads article HTML from the private `articles` bucket (`articles/{slug}.html`) and role-resume PDFs from the private `resumes` bucket (`resumes/{role_key}.pdf`). The article page reads that object and renders it in the same pane (in-page, for indexing), with the watermark mounted on `#html-renderer` so it does not cover the site chrome. Uploaded article `<style>` is still stripped so a post cannot restyle the public chrome. When `LOCAL_CONTENT_DIR` is set, `storage.New` uses that directory for the same object paths and does not call Supabase. Production leaves the variable unset. Admin preview uses that same reader after `RequireAdmin`: `GET /admin/resumes/{role_key}` streams the stored PDF, `GET /admin/articles/{id}/preview` renders the stored post (or `articles.body` when no file is set) in the article page, and `GET /admin/courses/{id}/preview` writes the stored lesson HTML. The course preview does not run `PrepareLesson`. The public learn route still does. None of these responses include a Storage URL. A warm instance may keep that login's access token in memory; dropping it only causes the next Storage request to sign in again. The token is not a content cache.
- **Profile photo and site logo are Storage objects too, migration 071** — not a repo-embedded static file (an earlier incarnation of the profile photo was `web/static/img/vishnu-photo.jpg`; that approach was retired specifically because `/static/` is served `Cache-Control: immutable, max-age=31536000`, so swapping the file in place at the same URL risked a browser or CDN edge serving the old bytes for up to a year). `core/handlers/admin.go`'s `AdminProfileSave` accepts an optional `photo_file`/`logo_file` multipart upload, sniffs the real format by magic bytes (`sniffImageExt` in `core/handlers/media.go` — never trusts a client-supplied `Content-Type` or filename), rejects anything that isn't JPEG/PNG/WEBP, and writes it to the private `profile-photo` or `site-logo` bucket at a fixed path (`profile/photo.<ext>` / `brand/logo.<ext>`) via the same `CourseStorage.Put`/`Delete` the resume upload already uses — if a new upload's extension differs from the stored one (a PNG replacing a JPEG), the old-extension object is deleted so two files for the same image never coexist. `profile.photo_object`/`logo_object` record the resulting path; empty means nothing uploaded yet. `GET /media/profile-photo` and `GET /media/logo` (public, no auth — `core/handlers/media.go`) stream whichever object is currently set, with `Cache-Control: no-cache` rather than a long max-age, so a replacement takes effect on the very next request with no risk of repeating the static-file incident. Home, About, the header (`header.html`), and the public layout's first-visit splash and between-page loader marks (`base.html`'s `#splash-loader`/`#page-loader-mark`, both gated on the same `BaseData.HasLogo`) all load the image from these fixed routes, never a bucket URL or a DB-stored URL string — with no logo uploaded, both loader marks fall back to the plain "VB" text badge they always showed. The admin form shows a live preview (client-side `URL.createObjectURL` on file selection) next to whichever image is already stored.
- **Database**: production is Supabase-hosted Postgres. On Vercel, `DATABASE_URL` must point at Supabase's **pooled (Transaction pooler) connection string**, not the direct connection — a serverless function opening/closing many short-lived connections against the direct connection exhausts Postgres's connection limit fast. `db.Open` logs a warning when the host looks like Supabase's direct connection (`*.supabase.co` without `pooler` in the hostname) rather than failing, so a plain local Postgres still boots. Local Compose uses a direct connection on purpose: one long-lived `cmd/server` process, not a fleet of serverless instances.
- **Connection pool**: a Vercel Go instance serves one request at a time, so the pool only has to cover the widest concurrent fan-out inside a single request — About's eight page lookups plus the two chrome lookups a signed-in visitor adds, currently `widestRequestFanOut+1` = 11 for both `MaxOpenConns` and `MaxIdleConns`. Matching idle to the ceiling means a warm instance does not re-establish connections on the widest page. If the fan-out grows, this constant has to grow with it, or one concurrent wave quietly becomes two.
- **Migrations**: `db.Migrate` reads `schema_migrations` once per cold start into a set, then skips already-applied files in memory. Recording a version uses `ON CONFLICT (version) DO NOTHING`: when several cold starts apply the same new file at once, the loser must not fail its insert, because a migration error is fatal for the whole app (this caused site-wide 500s when migration 082 shipped). `CREATE TABLE IF NOT EXISTS` does not cover that race, or a leftover composite type: both try to insert the table's row type and the loser hits `pg_type_typname_nsp_index` (SQLSTATE 23505). That took the site down on `095_email_templates.sql`. Migrations 094 and 095 check `pg_class.relkind` first, leave a real table (and its rows) in place, drop a composite type only when `relkind` is `c`, and catch `unique_violation` when the table is already visible. The previous per-file `SELECT count(*)` grew by one round trip with every migration added.
- **Local dev**: `docker compose up --build` is the local stack on Docker Desktop for Windows and for Mac (same Compose file; host paths use forward slashes). `docker-compose.yml` starts Postgres 16 (`db`, user/password/database `portfolio`) and the image built from `Dockerfile` (`cmd/server`). Copy `.env.example` to `.env` first. `DATABASE_URL` there is `postgres://portfolio:portfolio@db:5432/portfolio?sslmode=disable`: host `db` is the Compose service name, and the user, password, and database match `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`. Those four have to change together. Compose publishes Postgres on host port `POSTGRES_PORT` (default `5432`) and the app on `PORT` (default `8080`). `cmd/server/main.go` calls the same `app.Bootstrap()` as Vercel and runs `http.ListenAndServe`. Running `go run ./cmd/server` on the host against that database uses `localhost` in place of `db`. A missing `profile` row is an empty profile, not a 500; **Admin → Profile** inserts it on save. Migrations still insert no content. `docker/local-admin.sql` is the local placeholder admin (`admin@example.com` / `change-this-password`) and is not part of `Migrate()`. Apply it after the schema exists, or leave `ADMIN_BOOTSTRAP_EMAIL` set so every start adds the same email to the admin list. Compose bind-mounts `LOCAL_CONTENT_DIR` (default `../portfolio-local-content`, outside the repo) at `/content` and sets `LOCAL_CONTENT_DIR=/content` in the app container. Files there are `articles/{slug}.html`, `courses/{slug}.html`, `resumes/default.pdf`, and `resumes/{role-key}.pdf`. `.env.example` sets `SITE_URL=http://localhost:8080` and `GOOGLE_REDIRECT_URL=http://localhost:8080/login/google/callback` (a placeholder; register it on the OAuth client and paste the client id and secret). The entrypoint script is LF, and the image strips a Windows CR before it runs.

---

## 10. Security posture

- **`RESEND_API_KEY` and `BREVO_API_KEY`** live only in the host environment (Vercel, Sensitive). An empty value turns that provider off. From addresses, Reply-To, and the BCC recipient are not environment variables. `GET /internal/email/retry` uses the existing `CHATBOT_REINDEX_TOKEN`.
- CSP + `X-Content-Type-Options: nosniff` + `X-Frame-Options: DENY` + `Referrer-Policy: strict-origin-when-cross-origin` on every response (`middleware.SecurityHeaders`). CSP allows `'unsafe-inline'` for script/style (templates and embedded course-content pages both rely on inline blocks) but restricts *external* origins to only Razorpay's checkout domain — fonts are self-hosted (below), so `style-src`/`font-src` need no external allowance at all. `frame-src` allows `'self'` and Razorpay Checkout. The email preview handler replaces that CSP with `script-src 'none'` and a sandboxed iframe.
- All payment confirmation is server-side HMAC verification (`Razorpay.VerifySignature`) — never trusts a client-reported "payment succeeded."
- Admin and user sessions are fully independent JWTs in `HttpOnly`, `SameSite=Lax` cookies, `Secure` in production.
- Course-content access is an opaque random 32-byte hex bearer token, not a guessable ID. The learn page is the site chrome; the lesson loads from the gated `/learn/lesson` route (it answers only an iframe load with the buyer's session) in a unique-origin iframe, so the outer page source does not contain the lesson tags and the parent cannot read the lesson DOM. `vishnubalachandran.com` is drawn once, horizontally, centered in the pane (`core/storage/prepare.go`'s `WatermarkScript` — originally drawn three times along the pane diagonal; changed to a single centered line to read cleanly against the site's own type instead of cutting across it at an angle), in the site's font (`"Montserrat"`, matching `--font-sans`) and the same muted color token as the rest of the page's faint text (`--color-text-faint`, both light and dark values) — only inside the renderer; copied text is stamped with the viewer's identity.
- `/admin/*` is `noindex`ed at the HTTP-header level (`X-Robots-Tag`), not just via `robots.txt`.

---

## 11. Content vs schema — disaster recovery

The repo is **code only**. Migrations create tables, indexes, constraints and functions. They do not insert Vishnu's profile, skills, articles, courses, or anything else a visitor would read. UI chrome (headings, nav, button labels) stays in templates and handlers; that is not content.

**Live Postgres is the source of truth for content.** Edits happen in the admin console (`/admin/*`). After a content change, regenerate the snapshot from the live database:

```
psql "$DATABASE_URL" -AtX -v ON_ERROR_STOP=1 \
     -f core/db/gen_bootstrap_data.sql > portfolio_seed_data.sql
```

The output is stored in a Google Drive folder named **`Portfolio`**. Refer to that folder by name only — never record a Drive URL in this repo. The generator (`core/db/gen_bootstrap_data.sql`) stays in the repo because it is code; the snapshot does not. Generator scope: the content tables (`profile` incl. `rag_context`, `skill_groups`, `experiences`, `role_resumes`, `education`, `home_features`, `courses`, `software`, `articles`, `availability_rules`, `quiz_questions`, `certifications`, `publications`, `case_studies`) plus `page_seo` and `slug_redirects`. It carries the SEO columns on courses/articles/software (migration 085), `courses.preview_ready`/`tags`/`rag_context` (086), `software.licence_product`/`microsoft_store_url` (084, 092) and `quiz_questions.set_no`/`module` (088). It deliberately skips derived or activity data: `courses.outline` (cache), `articles.updated_at` (sitemap lastmod), `thumbs_up_count`, `quiz_attempts`, `quiz_attempt_grants`, licences, `auth_throttle`, users, purchases and bookings. Every statement is a single idempotent `WHERE NOT EXISTS` insert with the update folded in. When a migration adds or drops a content column, update the generator in the same commit, because a stale column list makes the whole snapshot fail (the dropped `courses.currency` did exactly that).

**Direction of truth.** Edit in admin → regenerate the snapshot. The snapshot is a bootstrap copy, not an authoring surface. Editing the SQL file and expecting the site to change is backwards.

**Disaster-recovery trade-off, stated plainly.** Checking out this repo and running migrations produces a complete, empty schema and a site that renders with no name, no skills, no articles. The repo **cannot rebuild content**. Recovering a wiped database means: apply the migration chain, then load `portfolio_seed_data.sql` from the `Portfolio` Drive folder (or restore a database backup). The owner admin email comes from `ADMIN_BOOTSTRAP_EMAIL` and the password is the `ADMIN_BOOTSTRAP_PASSWORD` variable, not the snapshot — the snapshot contains no admins and no credentials. The first admin account is created by `Bootstrap()` from `ADMIN_BOOTSTRAP_EMAIL`; its password is the `ADMIN_BOOTSTRAP_PASSWORD` variable, never a stored hash. Local Docker can also load `docker/local-admin.sql`, which inserts only the placeholder admin email from `.env.example` (`admin@example.com` / `change-this-password`). That file is not the snapshot and is not a migration. The article section of the snapshot includes `content_object` so a restored row still points at `articles/{slug}.html`.

Paid lesson HTML is not in the snapshot either. `courses.content_object` is the path (`courses/{slug}.html`) inside the private Storage bucket `course-content`. Article HTML uploaded from the admin console is a separate private bucket, `articles` (`articles.content_object`, path `articles/{slug}.html`). Role-resume PDFs are a third private bucket, `resumes`. The default download is the row `role_key = default` at `resumes/default.pdf`. A role-specific file is any other row, at `resumes/{role_key}.pdf`. The bytes live only in those buckets. Restoring the SQL without the bucket objects leaves every learn page, any uploaded article, and any resume download with a path and no file. Posts that still have `articles.body` set and an empty `content_object` restore with the SQL alone. A missing default resume row falls back to the generated PDF. The profile photo and site logo are the same story: two more private buckets, `profile-photo` (`profile.photo_object`, path `profile/photo.{ext}`) and `site-logo` (`profile.logo_object`, path `brand/logo.{ext}`) — restoring the SQL without those bucket objects leaves the header's logo and the Home/About hero photo pointing at a path with nothing there, same as a missing course lesson. Migration 055 creates `course-content` and a policy for `course-content@vishnubalachandran.com` when the `storage` schema exists; it does not create that login or upload the files. Migration 056 adds `articles.content_object`. Migration 057 creates the `articles` bucket and the same login's policy on it. Migration 058 creates `role_resumes` (with row-level security and no content rows) and the `resumes` bucket with that same login's policy. Migration 071 creates `profile-photo` and `site-logo` the same way, drops the old `profile.photo_url` column (the repo-embedded-file approach it replaced), and adds `photo_object`/`logo_object`.

`Migrate()` only executes files that exist and are not yet recorded in `schema_migrations`. It never re-reads an already-applied file, and it never checks that a recorded filename still has a matching file on disk. Editing or deleting a historical migration is therefore a no-op against production once that version is recorded; it only changes what a fresh database gets.

---

## 12. AI chatbot — RAG over site content (`core/ai`, `core/repo/chatbot_chunks.go`, `core/handlers/chatbot.go`)

A floating widget on every page (`chatbot-widget`, gated on `BaseData.ChatbotEnabled`) answers visitor questions about Vishnu's background, courses, and articles using retrieval-augmented generation — not a canned FAQ bot, and no paid API dependency. "AI Assistant" is the name used throughout (widget label, panel header, the dedicated page below) — a deliberately generic, commonly-searched term rather than site-specific branding, chosen so the feature can pick up generic "AI assistant" search traffic too, not just branded searches.

**One shared widget partial, two shells (`templates/layout/chatbot_widget.html`).** The widget's markup used to live inline in `base.html`; it's now `{{define "chatbot_widget"}}`, parsed into every public page (added to `publicLayout` in `view.go`, same as `skills_grid.html`/`subscribe_form.html`) so both call sites stay byte-identical rather than drifting (CLAUDE.md rule 1). `base.html` renders it as the floating bubble on every page except one: `{{if not .ChatbotFullPage}}{{template "chatbot_widget" .}}{{end}}`.

**The opening greeting is injected by `main.js`, not server-rendered.** `#chatbot-messages` renders empty from the template; on load, `main.js` appends the greeting via `appendMessage` unless a transferred conversation (see below) already populated it. This changed Oct 2026 after a live stats/listing audit found the greeting's static text — present in every page's raw HTML since the widget sits on every page — occasionally winning over a page's actual meta description as the Google search snippet (seen on `/software`, which also had a too-long meta description at the time; both were fixed together). The widget was already entirely JS-driven (hidden by default, opened by a click handler), so moving the one static string client-side cost nothing functionally. `AIAssistantPage` (`GET /ai-assistant`, `chatbot.go`) renders the *same* partial as that page's entire main content, with `BaseData.ChatbotFullPage = true`, inside the normal page chrome (`ai_assistant.html` uses the standard `base.html`/header/footer layout like any other page — this is a real page, not a popup or iframe). The template itself switches shells off that one bool: full-page mode drops the floating toggle button and the panel's `hidden` attribute (there's nothing to show/hide — the whole page already is the chat) and gets a `chatbot-widget--full` CSS class instead of fixed positioning. `main.js`'s event wiring needs no branching for this: `chatbotToggle`/`chatbotClose` are simply `null` in full-page mode (no toggle/close elements rendered), so those two `addEventListener` calls are wrapped in `if (chatbotToggle)`/`if (chatbotClose)` null checks, and the full-page chat auto-focuses its input on load instead (`if (!chatbotToggle) chatbotInput.focus()`). The floating panel's header also gets a small expand-icon link to `/ai-assistant` (omitted in full-page mode, where it would be redundant) — that's the "open full screen" affordance.

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

**Incident: Vercel's own Cron Jobs can't run this on the Hobby plan, so a GitHub Actions workflow does (Oct 2026).** The first version of this design configured a Vercel Cron Job in `vercel.json`'s `"crons"` array at `*/5 * * * *`. Every deploy after that silently stopped working: Vercel's deploy validation rejected it outright (`"Hobby accounts are limited to daily cron jobs ... Upgrade to the Pro plan to unlock all Cron Jobs features"`), which doesn't show up as a runtime error or a failed-but-visible deployment in the usual places -- it blocks the deployment from being created at all, so `git push` appeared to succeed while production silently stayed on the previous commit for several pushes in a row. A once-daily cron would be Hobby-compliant but defeats the purpose (a save at 9am wouldn't be reflected until the next day). The fix: `.github/workflows/chatbot-reindex.yml` runs on GitHub's own free Actions scheduler every 5 minutes (`workflow_dispatch` also lets it be triggered by hand) and just calls `GET /internal/chatbot/cron-reindex` with `Authorization: Bearer ${{ secrets.CHATBOT_REINDEX_TOKEN }}` -- the repo is public so Actions minutes are free and the workflow is unrestricted (repo rule 13; GitHub runs the schedule from `main` anyway) -- the endpoint itself didn't need to change at all, since it was never actually Vercel-specific code, just reached by a different scheduler. The GitHub Actions secret `CHATBOT_REINDEX_TOKEN` must be set on this repo (Settings → Secrets and variables → Actions) to the same value as the Vercel env var of the same name. **Lesson for future maintenance:** a Vercel platform feature's availability can depend on the account's plan tier in ways that fail at deploy time with no visible error on the deployment list -- if a push stops producing a new deployment with no explanation, check whether `vercel.json` changed and try a direct `create_deployment` call (or `vercel deploy` locally) to surface the actual validation error, rather than assuming the webhook is just slow.

**Agent-triggered reindex (`APIChatbotReindex`, POST `/api/chatbot/reindex`, public route, bearer-token-gated).** A coding agent that edits course or article content directly against Supabase — a DB row, or an HTML object in the `course-content`/`articles` Storage buckets — bypasses the admin UI's save/delete handlers entirely, so the auto-reindex above never fires for that change. This endpoint gives such an agent the same trigger without an admin browser session: `Authorization: Bearer <CHATBOT_REINDEX_TOKEN>` (`crypto/subtle.ConstantTimeCompare`, not `==`, to avoid a timing side-channel on the token) runs the identical `RebuildChatbotIndex` synchronously and responds `200` with the resulting chunk count once it's actually done (see the incident note above for why this doesn't respond `202`/"started" before the work finishes). The endpoint itself returns `404` whenever `CHATBOT_REINDEX_TOKEN` is unset — same "absent until configured" pattern as `NVIDIA_API_KEY` gating the chatbot widget — so there's no unauthenticated reindex trigger exposed by default on a fresh deploy.

**`CronChatbotReindex` (`GET /internal/chatbot/cron-reindex`) reuses the same token for a different caller: a scheduled job, not an agent.** It checks the incoming bearer token against `a.Cfg.ChatbotReindexToken` — the exact same comparison `APIChatbotReindex` already does — rather than adding a second secret for what is, functionally, the same permission ("you may trigger a reindex") granted to a different caller. `.github/workflows/chatbot-reindex.yml` (see the incident note above for why this is a GitHub Actions workflow and not a Vercel Cron Job) sends that same `CHATBOT_REINDEX_TOKEN` value, stored as a GitHub Actions secret of the same name, as its `Authorization` header every 5 minutes. Unlike `APIChatbotReindex`, this handler calls `ChatbotChunkRepo.ConsumePendingReindex` first and only runs `RebuildChatbotIndex` when that reports true -- an agent calling the API endpoint directly wants a rebuild *now, unconditionally*; the scheduled job's whole purpose is to do nothing on the common tick where nothing changed, so it doesn't spend NVIDIA free-tier budget on no-op runs every 5 minutes, 24/7.

**Answering a question (`Chat`, POST `/chat`, public).** Embed the question (`inputType="query"`) → `ChatbotChunkRepo.Search` for the 5 closest chunks, scoped to the asker's own unlocked courses → **intent/relevance gate** (below) → build a system prompt that includes only that retrieved text and instructs the model to say "I don't have that information" rather than invent facts, prices, or dates → `Client.Chat` → return the reply plus the deduplicated source URLs the retrieved chunks came from. The widget (`web/static/js/main.js`) renders the reply and clickable source links in a floating panel; its markup is entirely absent from the rendered page (not just hidden by CSS) when `ChatbotEnabled` is false, and it's present on every public page (including a gated lesson page itself, `course_learn.html`) since they all share the same `base.html` layout.

**Conversational context (client-held history, no server-side session store).** The widget keeps a plain in-memory JS array, `chatHistory`, of `{role, content}` turns for the lifetime of the page (`web/static/js/main.js`) and sends it as `history` in every `/chat` request body, alongside the new `message`. There is deliberately no server-side conversation store — per CLAUDE.md rule 3, this app has no in-process state that survives between requests and a serverless fleet has no shared memory between instances, so a session-keyed history table would need the same kind of plumbing `chatbot_rate_limits` already has just to hold what the browser can hold for free. The server still never trusts client-supplied history blindly: `sanitizeHistory` (`chatbot.go`) drops any turn whose `role` isn't `"user"`/`"assistant"`, trims and caps each turn's content at `chatbotMaxHistoryChars` (500), and keeps only the most recent `chatbotMaxHistoryTurns` (6) turns — bounding both the prompt size and the cost of a pathological client payload. This fixed a real reported failure: a visitor asked a follow-up ("As lead?") after an initial question about AI experience, and the bot — having no memory of the prior turn at all — answered as if "As lead?" were a standalone, context-free question and fell back to its "I don't have that information" default. Two places needed the history, for different reasons: `historySearchQuery` folds only the most recent *user* turn (never the assistant's reply, never older turns) into the text that gets embedded for retrieval — enough to disambiguate a short follow-up like "As lead?" into something `Search` can match against, without diluting the embedding with unrelated older context; the full sanitized history, separately, is passed to `Client.Chat`'s `messages` array so the model actually sees the conversation when generating its answer. `Client.Chat` (`core/ai/nvidia.go`) builds `[system, ...history, user]` before marshaling — the system prompt's own text was extended to tell the model the conversation is continuous, so it should resolve a short follow-up against the prior turn rather than treating every message as the first.

**Intent gate before the generation call.** `ChatbotChunkRepo.Search` returns each chunk's pgvector cosine distance from the query (0 = identical, 2 = opposite). If the single closest chunk's distance exceeds `chatbotRelevanceThreshold` (0.75, hand-picked, not measured against a labeled set -- revisit if real traffic shows it too strict or too loose), `Chat` returns a fixed "I can only answer questions about Vishnu..." reply and returns *before* calling `Client.Chat` at all. This is deliberately not a second LLM call: the embedding + retrieval step already has to run to answer any in-scope question, so the distance it produces is free signal, and skipping the context-stuffed generation call on an off-topic question is where the actual token cost lives. A plainly off-topic question (general trivia, something unrelated to Vishnu/his courses/his articles) never reaches the LLM.

---

*Last updated: October 2026, alongside the RAG chatbot and AI/GenAI course work. Companion doc: `docs/FUNCTIONALITY.md`. If you're an AI agent about to modify this codebase, read both files first — they're written specifically so you don't have to re-derive this context from the source tree.*

## 13. Design system and the course reader

The visual system (tokens, type, motion, components) is documented in `docs/DESIGN.md`; `web/static/css/style.css` is one file built on those tokens, with legacy `--color-*` aliases kept for uploaded lesson/article CSS. Fonts are self-hosted variable woff2 (Inter, JetBrains Mono, plus Cormorant), served from `/static/fonts/` with `Access-Control-Allow-Origin: *` (`staticCache` in `routes.go`) because the sandboxed lesson iframe has an opaque origin and fonts load in CORS mode.

**Course reader** (`GET /courses/{slug}/learn`, `core/handlers/learn.go`, `course_learn.html`). `BaseData.Immersive` puts the page in reader mode (`body.is-immersive` hides the promo bar, footer and floating widgets). The lesson loads from its own gated route, `GET /courses/{slug}/learn/lesson` (admin preview: `/admin/courses/{id}/preview/lesson`), into `<iframe sandbox="allow-scripts">`. It answers an iframe load; a page navigation or view-source (`Sec-Fetch-Dest: document`) gets a 403, and a browser too old to send the header is let through. `storage.PrepareLesson` now also injects `lessonRuntimeCSS`/`lessonRuntimeJS` (`core/storage/lesson_runtime.go`): the lesson reads its own `h2`/`h3` headings and posts `lesson-toc` and `lesson-progress` to the parent, posts `lesson-run` when a reader presses Run on a code block, and accepts `lesson-goto` and `reader-config` from the parent (checked against `window.parent`). The parent checks `e.source === frame.contentWindow`. The last section read is kept in `localStorage` per course.

**Lesson document and `unsafe-eval` (Oct 2026).** Four lessons (`java-fundamentals`, `go-portfolio-from-scratch`, `java-sse-ultimate-guide`, `ai-repo-rules-rag-context`) contain code labs that compile the learner's code with `new Function`. An inline `srcdoc` document inherits the site CSP, which has no `'unsafe-eval'`, so those labs were blocked. The lesson is therefore served as its own response with its own policy (`lessonCSP` in `learn.go`): `sandbox allow-scripts` (unique origin, no cookies, no access to the parent), `default-src 'none'`, `connect-src 'none'`, `frame-src 'none'`, `form-action 'none'`, `script-src 'unsafe-inline' 'unsafe-eval'`. The learner's code can run but cannot reach any network, storage or the site. The site-wide CSP in `core/middleware/security.go` still has no `'unsafe-eval'` (`TestSiteCSPHasNoUnsafeEval`). Do not add `'unsafe-eval'` anywhere else; when a lesson needs to run code, use the editor below or keep it inside a lesson document.

**Snippet Run/Copy (`core/storage/lesson_runtime.go`).** The lesson runtime adds a Copy and a Run button to every `<pre>`; Run is hidden unless the reader reports a compiler for the snippet's language, and a `<pre data-norun>` never gets one (whole-project snippets). Copy and Run read the block with `innerText || textContent` because `innerText` is empty for blocks in non-visible sections. The runtime also handles in-lesson `#id` links itself (click handler scrolls inside the sandboxed lesson document), which the "Setting up the examples" links rely on.

**Compiler drawer.** `JDOODLE_EMBED_KEYS` ("java=…,python=…,go=…", `config.ParseEmbedKeys`) lists JDoodle embeds; each JDoodle embed is tied to one saved JDoodle project, so it fixes the language. The owner sets it in Vercel as a Sensitive variable. Key values are never in the repo, docs, commit messages or memory (CLAUDE.md rule 5). **Keys are never in page HTML either.** The template lists languages only. When the drawer opens, it calls `GET /courses/{slug}/learn/compiler?lang=` (`CourseCompilerEmbed`), which returns `{src}` only to a verified purchaser or an admin (`courseAccess`). The call must also carry the `X-Requested-With: vb-compiler` header and pass the `Sec-Fetch-Site` same-origin check, and the response is `no-store`. Limitation: the buyer's browser loads the embed straight from jdoodle.com, so the address shows in that buyer's developer tools. No third-party embed can avoid that. If a key is abused, delete the embed in JDoodle and add a new key. With none set the drawer and the lesson Run buttons don't render. The drawer creates `<div data-pym-src="https://www.jdoodle.com/embed/v1/<key>">` and loads `jdoodle-pym.min.js` on first open, outside the sandboxed lesson (a nested iframe would inherit the sandbox). The CSP allows `https://www.jdoodle.com` in `script-src` and `frame-src` only. The default language comes from `models.Course.CodeLanguage()` (tags, then keywords, then Java). Run switches the drawer to the snippet's language and runs it. If that embed is already loaded and has posted `{onLoad}`, the reader sends `setCode` then `execute` through `pym.autoInitInstances[…].sendMessage`. Otherwise it inserts a fresh holder whose text is the snippet (`jdoodle-pym.min.js` answers the embed's `getScript` request with the holder's text), re-inserts the script, and sends `execute` on `onLoad`. The script registers embeds only when it runs, so every language switch re-inserts it. Run buttons appear only for languages with a key (`reader-config.langs`). Live keys (Oct 2026): java, python, go, bash. Each run counts against the owner's JDoodle plan; the free tier allows 1,000 runs a day. **Server-side runner tried and dropped (Oct 2026):** driving the embed from a headless browser on Vercel makes JDoodle show a reCAPTCHA, so nothing runs. That is an anti-bot check and must not be bypassed; the embed stays in the buyer's own browser. The address can be kept out of the page, the script and the DOM source but not out of that buyer's network panel. If a key leaks, delete that embed in JDoodle and add a new one.

**"What's inside" outline.** `storage.LessonOutline` extracts a lesson's `h2` titles (or `h3` when there are none), strips numbering prefixes, skips the lesson's own navigation headings ("On this page", "Contents" — `models.IsNavigationHeading`, also applied when `Course.OutlineItems` renders a stored outline and by the reader's lesson runtime), drops duplicates and caps at 40. It is written on every lesson upload (single form and bulk) and lazily backfilled by `CourseDetail` the first time a course with a stored lesson has an empty outline (best effort, never fails the page, never bumps `updated_at`).

**Preview builder (`core/handlers/course_preview_build.go`, migration 091).** One path computes `courses.outline` and `courses.preview_ready` from the stored lesson: `previewFromLesson(raw)` composes `storage.LessonOutline` and `storage.LessonPreview` (no second implementation of either) and is used by the single-lesson upload, the bulk upload and the builder; `buildCoursePreview` reads the lesson from Storage and persists via `CourseRepo.UpdateOutline` (which also stamps `preview_checked_at`). The course page calls `backfillCoursePreview` when `needsPreviewBuild` (a lesson exists and the outline is empty or `preview_ready` is false); `CourseRepo.ClaimPreviewBuild` is an atomic `UPDATE ... WHERE preview_checked_at IS NULL OR older than 6 h`, so concurrent serverless instances do one Storage read between them and a lesson with nothing to preview is retried at most every 6 hours (no in-process state). A slug rename clears the three columns so the next view rebuilds. The admin POST routes `/admin/courses/rebuild-previews` and `/admin/courses/{id}/rebuild-preview` run `rebuildPreviews` (4 lessons at a time) and render the per-course result on the courses list.

**Course slug rename and redirects (migration 086).** Changing the slug in Admin -> Courses -> Edit runs `App.renameCourse`: it refuses a slug another course uses; copies the Storage object `courses/{old}.html` to `courses/{new}.html` (the old object is kept as a rollback copy; works with `LOCAL_CONTENT_DIR`); then `CourseRepo.UpdateRenaming` saves the course, repoints `content_object`, clears `outline` and `preview_ready` (both rebuilt by the next sales-page view) and upserts `slug_redirects` rows for `/courses/{old}` plus `/preview`, `/learn`, `/quiz` and `/certificate`, all in one transaction; the save handler then flags the chatbot reindex and shows the report on the course list. Re-running it is harmless (the copy overwrites, the rows are upserted; a row that pointed at the old path is repointed, and renaming back removes the circular row). Redirects are read only by `App.NotFound` (`redirectRenamed`), i.e. on a request that was about to 404, so no live route pays a query; only GET and HEAD get the 301, and the query string is kept.

**Free preview.** `storage.LessonPreview` returns the part of a stored lesson before the `<!-- vb:preview-end -->` marker (`storage.PreviewEndMarker`). `NormalizeLesson` leaves the marker alone (tested). With no marker the preview is the first section only (everything before the second `<h2>`, or `<h3>` if the lesson has no `<h2>`); a lesson with neither has no preview. The lesson's trailing `<script>` blocks are kept so interactive pieces in the preview still run. The cut is applied server-side in `writeLessonDocumentCut`, so nothing after it is sent to a browser.

**Content normalizer.** `storage.NormalizeLesson` (`core/storage/normalize.go`) runs on every lesson and article upload (course form, bulk upload, article form) and offline via `cmd/lessonfmt`. It maps a lesson's own `:root` palette onto the site tokens as `--l-*` properties, removes in-lesson tables of contents, strips `<h2>` numbering, and labels code blocks with their language. `<!-- vb:theme dark -->` opts a dark app-style lesson into the site's dark palette in both schemes. It is idempotent: it stamps `<!-- vb:normalized v1 -->` and skips files that already carry it. The strategy is in `docs/DESIGN.md` ("Content updates").

**Bulk lesson upload** (`GET/POST /admin/courses/bulk-upload`, `admin_course_bulk.go`). One zip (≤ 8 MB; Vercel caps a request body at 4.5 MB in practice) of `{slug}.html` files; each matches an existing course by file name, is stored at `courses/{slug}.html`, gets its outline re-derived, and the chatbot reindex flag is set once. Unknown slugs are reported and skipped.


## Quiz attempt cap (migration 088)

The cap of 5 attempts per learner per course lives in Postgres, not Go. `quiz_attempts.attempt_no` is unique per `(user_id, course_id)`, and the `quiz_attempt_cap_guard` trigger raises `check_violation` when `attempt_no > 5 + COALESCE(quiz_attempt_grants.extra, 0)`, after taking `pg_advisory_xact_lock` on the same key `QuizAttemptRepo.Start` uses. `Start` locks, resumes any open attempt, otherwise inserts `max(attempt_no)+1` with the chosen question ids; the repo maps the DB error to `ErrNoAttemptsLeft`. A grant is therefore the only way past 5, and it can only be written by the admin handler. `models.QuizMaxAttempts` mirrors the literal 5 in the trigger: change both together. Legacy visitor attempts are numbered by migration 088 (a learner with more than 5 old attempts starts locked until granted). Serverless note: everything is a transaction on the pooled connection; no in-process state.

**Visitor field validation.** `core/handlers/formcheck.go` holds the one set of rules for text visitors type: `fieldSpec`/`checkField`/`firstFieldError` over the kinds name, line, text, phone and email, built on `validPersonName`, `englishText`, `validPhone` and ASCII `validEmail`. Handlers declare fields and never re-implement the checks (CLAUDE.md rule 14).
