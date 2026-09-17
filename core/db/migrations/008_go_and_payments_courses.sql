-- Two more courses: building a portfolio in Go from scratch, and
-- integrating payments/bookings into it. Held unpublished until their
-- content (core/content/courses/go-portfolio-from-scratch.html and
-- payment-integration-razorpay.html) lands and is verified, then flipped
-- live in a follow-up migration — same pattern as the Kafka course.
INSERT INTO courses (slug, title, summary, description, level, duration, price_cents, currency, is_paid, min_price_cents, min_price_cents_inr, content_key, is_published, sort_order) VALUES
('go-portfolio-from-scratch', 'Build Your Own Portfolio in Go: Beginner to Expert',
 'Learn Go from scratch and build a real, deployable personal portfolio site: html/template, plain CSS/JS, a Supabase Postgres database, and shipping it on Vercel with your own domain.',
 'A complete, beginner-friendly path from zero Go knowledge to a real, deployed portfolio site: Go language fundamentals, net/http and Go''s own html/template for server-rendered pages, a plain-CSS/vanilla-JS frontend with no build step, connecting a Supabase Postgres database with pgx, deploying as a Go serverless function on Vercel, and linking a custom domain. Includes runnable Go-syntax coding labs and hands-on deployment exercises.',
 'Beginner to Advanced', 'Self-paced', 1500, 'USD', TRUE, 1500, 120000, 'go-portfolio-from-scratch', FALSE, 7),
('payment-integration-razorpay', 'Integrating Payments, Courses & Bookings Into Your Portfolio (Razorpay)',
 'Add real monetization to your site with Razorpay: selling paid courses, taking paid bookings without double-booking, server-side signature verification, and what to do yourself vs. what AI can build for you.',
 'A security-focused, practical guide to integrating Razorpay into any backend (Go examples throughout): the Orders API and Checkout.js flow, why server-side HMAC signature verification is non-negotiable, a full worked "sell a course" walkthrough, a full worked "sell a booking slot" walkthrough (including the database-level fix for double-booking races), regional/flexible pricing, checkout UX that doesn''t feel amateur, and a clear manual-steps-vs-AI-assistable breakdown with example prompts. The Go portfolio course is a helpful optional prerequisite, not a requirement — the concepts apply to any stack.',
 'Intermediate to Advanced', 'Self-paced', 1800, 'USD', TRUE, 1800, 150000, 'payment-integration-razorpay', FALSE, 8)
ON CONFLICT (slug) DO NOTHING;
