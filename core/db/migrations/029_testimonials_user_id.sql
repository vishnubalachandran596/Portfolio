-- Lets a logged-in visitor submit their own testimonial (always unpublished
-- until an admin approves it) instead of every testimonial being
-- admin-authored.
ALTER TABLE testimonials ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_testimonials_user ON testimonials(user_id);
