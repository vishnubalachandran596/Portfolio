-- Paid courses: an optional pay-what-you-want (with a floor) unlock, plus
-- a purchases ledger and per-course gated content.

ALTER TABLE courses ADD COLUMN IF NOT EXISTS is_paid BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE courses ADD COLUMN IF NOT EXISTS min_price_cents INT NOT NULL DEFAULT 0;
ALTER TABLE courses ADD COLUMN IF NOT EXISTS content_key TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS purchases (
    id                SERIAL PRIMARY KEY,
    course_id         INT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    email             TEXT NOT NULL,
    amount_cents      INT NOT NULL,
    currency          TEXT NOT NULL DEFAULT 'USD',
    status            TEXT NOT NULL DEFAULT 'created', -- created | paid | failed
    razorpay_order_id TEXT NOT NULL UNIQUE,
    razorpay_payment_id TEXT NOT NULL DEFAULT '',
    access_token      TEXT NOT NULL UNIQUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at           TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_purchases_course ON purchases (course_id, status);
CREATE INDEX IF NOT EXISTS idx_purchases_access_token ON purchases (access_token);

ALTER TABLE purchases ENABLE ROW LEVEL SECURITY;

-- The Java SSE interview-prep guide: the first real paid course. Its full
-- interactive content is embedded server-side (core/content) and served
-- only to a verified purchaser at /courses/<slug>/learn — the summary and
-- description below are the public teaser shown before purchase.
INSERT INTO courses (
    slug, title, summary, description, level, duration,
    price_cents, currency, is_paid, min_price_cents, content_key,
    is_published, sort_order
) VALUES (
    'java-sse-ultimate-guide',
    'Java SSE Ultimate Learning Guide',
    'A complete interview-prep lab: 138 DSA problems with an in-browser code runner, 35 LLD designs, HLD, and Java backend deep-dives.',
    'Everything needed to prepare for a Java Software/Senior Software Engineer interview, in one interactive guide:

Part 0 — Foundations: a conceptual Q&A bank, a DP mental model, a DSA pattern map, load balancer/proxy/API gateway concepts, a database scaling cheatsheet, deployment strategy, and API/JWT/idempotency fundamentals.

138 DSA problems, each with a runnable code lab (edit and run against real test cases directly in the browser), a step-by-step walkthrough, and multiple test cases per problem — arrays, strings, trees, graphs, dynamic programming, and more.

35 Low-Level Design (LLD) problems — object-oriented system designs like a car rental system, worked through in depth.

High-Level Design (HLD) expansion and backend high-yield topics, including HashMap internals in modern Java.

This is a genuinely large body of work — a real interview-prep course, not a teaser.',
    'Intermediate to Advanced', 'Self-paced',
    500, 'USD', TRUE, 500, 'java-sse-ultimate-guide',
    TRUE, 0
) ON CONFLICT (slug) DO NOTHING;
