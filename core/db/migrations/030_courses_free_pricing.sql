-- All courses move to a "contribute as you like" model: no hard minimum
-- by default. Zeroing existing minimums here; individual courses can still
-- have a minimum re-set later via /admin/courses.
UPDATE courses SET min_price_cents = 0, min_price_cents_inr = 0;
