-- Verified-buyer course reviews: a logged-in account with a paid purchase
-- for a course (any amount, including a free/100%-off enrollment -- the
-- "purchases" row is what makes someone a buyer here, not the amount) may
-- leave a 1-5 star rating plus an optional written comment, once per
-- account per course (re-submitting edits the same row rather than adding
-- a second one).
--
-- The rating/comment content is never shown publicly -- only the total
-- count of ratings a course has is public (CourseReviewRepo.CountByCourse,
-- rendered on the course detail page). The rating value and comment text
-- are admin-only, read from /admin/courses/{id}/reviews.
CREATE TABLE IF NOT EXISTS course_reviews (
    id         BIGSERIAL PRIMARY KEY,
    course_id  BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating     INT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_course_reviews_course_id ON course_reviews(course_id);

ALTER TABLE course_reviews ENABLE ROW LEVEL SECURITY;
