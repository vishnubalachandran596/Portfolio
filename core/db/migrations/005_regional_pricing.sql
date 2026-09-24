-- Regional pricing for paid courses: a separate INR minimum for visitors
-- detected as being in India, alongside the existing international
-- minimum (min_price_cents / currency).
ALTER TABLE courses ADD COLUMN IF NOT EXISTS min_price_cents_inr INT NOT NULL DEFAULT 0;
