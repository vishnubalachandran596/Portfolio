-- Redesign: visitor accounts (required to book consultancy or buy a
-- course), tiered course pricing across a real course catalog, and paid
-- consultancy bookings.

-- Visitor accounts, separate from the admins table. An account whose
-- email matches an admin's email is *also* granted an admin session on
-- login (see handlers/auth.go) — one login form, role decided by email.
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- Consultancy bookings now carry payment: a slot is provisionally
-- reserved (status='pending_payment') the moment a Razorpay order is
-- created — the existing UNIQUE(slot_start) constraint stops two buyers
-- racing for the same slot — then flips to 'confirmed' once payment is
-- verified. The availability query treats a pending reservation older
-- than 20 minutes as free again, so an abandoned checkout doesn't need a
-- separate cleanup job.
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS user_id INT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS amount_cents INT NOT NULL DEFAULT 0;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT 'USD';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS razorpay_order_id TEXT NOT NULL DEFAULT '';
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS razorpay_payment_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_bookings_razorpay_order ON bookings (razorpay_order_id) WHERE razorpay_order_id <> '';

-- The Java SSE guide keeps its own tier now that a dedicated entry-level
-- Java Fundamentals course exists below it.
UPDATE courses SET price_cents = 900, currency = 'USD', min_price_cents = 900, min_price_cents_inr = 75000
WHERE slug = 'java-sse-ultimate-guide';
UPDATE courses SET sort_order = 6 WHERE slug = 'java-sse-ultimate-guide';

-- Retire the placeholder/teaser courses — no real content, superseded by
-- the fully-built courses below.
DELETE FROM courses WHERE slug IN ('event-driven-microservices', 'resilient-system-design', 'engineering-leadership', 'ai-assisted-engineering');

INSERT INTO courses (slug, title, summary, description, level, duration, price_cents, currency, is_paid, min_price_cents, min_price_cents_inr, content_key, is_published, sort_order) VALUES
('java-fundamentals', 'Java Fundamentals for Interviews & Production Code',
 'Core Java done right: language foundations, OOP, collections, generics, streams, concurrency basics, and writing clean production code.',
 'The on-ramp before the advanced Java SSE guide: JVM internals, OOP principles done with real-world-shaped examples, the collections framework, generics, Java 8+ streams and lambdas, concurrency fundamentals, memory and garbage collection, and the habits that separate clean production Java from code that merely compiles. Includes hands-on runnable coding labs with real test cases.',
 'Beginner to Intermediate', 'Self-paced', 600, 'USD', TRUE, 600, 50000, 'java-fundamentals', TRUE, 1),
('technical-architecture-design', 'Technical Architecture & Design Principles',
 'Moving from writing good code to designing good systems: SOLID in practice, architecture styles, event-driven design, data architecture, and running architecture reviews.',
 'A practical architecture course grounded in real banking and fintech systems: core principles (coupling, cohesion, SOLID with real-world-shaped examples), layered vs hexagonal vs clean architecture, monolith vs microservices vs modular monolith, event-driven architecture and the outbox pattern, data architecture decisions, API design as an architectural choice, architecture decision records, and leading architecture reviews. Includes interactive design exercises.',
 'Intermediate to Advanced', 'Self-paced', 1200, 'USD', TRUE, 1200, 100000, 'technical-architecture-design', TRUE, 2),
('system-design-deployment', 'System Design & Deployment Patterns',
 'Scaling fundamentals, caching, database scaling, reliability patterns, and deployment strategies — with two full worked system-design walkthroughs.',
 'Practical, correct mental models for system design interviews and real production systems: back-of-envelope estimation, caching strategies, database scaling, asynchronous processing, circuit breakers and retries with jitter, blue-green/canary deployments, and observability as a design input — closing with two full worked system-design case studies. Includes interactive simulators.',
 'Intermediate to Advanced', 'Self-paced', 1200, 'USD', TRUE, 1200, 100000, 'system-design-deployment', TRUE, 3),
('kafka-messaging-caching', 'Kafka, Messaging & Caching: Event Streaming with Redis',
 'Kafka internals, delivery semantics, the outbox and saga patterns, the actor model with Akka, and Redis as a cache and beyond.',
 'Deep, mechanism-level coverage of event streaming and caching: Kafka topics/partitions/offsets, delivery semantics done precisely, replication and reliability, schema evolution, Kafka Streams concepts, the outbox pattern and choreographed sagas, the actor model with Akka and when it genuinely fits, and Redis for caching and beyond — with an honest, direct comparison of Redis pub/sub versus Kafka. Includes interactive simulators.',
 -- Published in a follow-up migration once core/content/courses/kafka-messaging-caching.html lands (content generation in progress).
 'Advanced', 'Self-paced', 1800, 'USD', TRUE, 1800, 150000, 'kafka-messaging-caching', FALSE, 4),
('resilient-scalable-systems', 'Resiliency & High-Scale Application Design',
 'The advanced capstone: failure modes, resilience patterns, consensus, data consistency trade-offs, capacity planning, and operating resilient systems at scale.',
 'The most advanced course on the site: failure modes in distributed systems, resilience patterns in depth (circuit breakers, bulkheads, load shedding), consensus and coordination at a conceptual level, data consistency trade-offs and sagas, scaling stateful systems, capacity planning and load testing, chaos engineering and blameless postmortems, closing with a full worked payments/ledger case study. Includes interactive simulators.',
 -- Published in a follow-up migration once core/content/courses/resilient-scalable-systems.html lands (content generation in progress).
 'Advanced', 'Self-paced', 2400, 'USD', TRUE, 2400, 200000, 'resilient-scalable-systems', FALSE, 5)
ON CONFLICT (slug) DO NOTHING;
