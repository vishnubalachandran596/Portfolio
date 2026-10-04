-- Two additions, both idempotent via IF NOT EXISTS:
--
-- promo_codes.banner_text: an optional override for the sitewide promo
-- banner's copy (e.g. "First course free using promocode X on signup"
-- instead of the auto-generated "Use code X for Y% off"). Empty keeps
-- the auto-generated line -- see promo_banner.html.
--
-- purchases.promo_text: a snapshot of the promo code's own banner_text
-- (or description, if no banner text was set) taken at redemption time
-- and shown on the printable receipt, so the receipt keeps showing what
-- the code actually said even if the code is later edited or deleted.
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS banner_text TEXT NOT NULL DEFAULT '';
ALTER TABLE purchases ADD COLUMN IF NOT EXISTS promo_text TEXT NOT NULL DEFAULT '';
