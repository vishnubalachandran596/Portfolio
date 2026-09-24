-- Indexes that match what core/repo actually queries, plus removal of two
-- that duplicate a unique index byte for byte.
--
-- Idempotent: CREATE INDEX IF NOT EXISTS / DROP INDEX IF EXISTS throughout.

CREATE INDEX IF NOT EXISTS idx_articles_published_date
    ON articles (published_date DESC NULLS LAST, id DESC)
    WHERE is_published = TRUE;

CREATE INDEX IF NOT EXISTS idx_articles_thumbs
    ON articles (thumbs_up_count DESC, published_date DESC NULLS LAST, id DESC)
    WHERE is_published = TRUE;

CREATE INDEX IF NOT EXISTS idx_article_thumbs_up_user    ON article_thumbs_up (user_id);
CREATE INDEX IF NOT EXISTS idx_quiz_attempts_user        ON quiz_attempts (user_id);
CREATE INDEX IF NOT EXISTS idx_bookings_promo_code       ON bookings (promo_code_id);
CREATE INDEX IF NOT EXISTS idx_purchases_promo_code      ON purchases (promo_code_id);
CREATE INDEX IF NOT EXISTS idx_promo_code_courses_course ON promo_code_courses (course_id);

DROP INDEX IF EXISTS idx_purchases_access_token;
DROP INDEX IF EXISTS idx_bookings_slot_start;
