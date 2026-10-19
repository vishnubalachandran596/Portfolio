-- Quiz engine: five non-overlapping question sets per course bank, and a
-- hard cap of 5 attempts per learner per course enforced in the database.
--
-- Everything here is idempotent (IF NOT EXISTS / pg_constraint guards /
-- CREATE OR REPLACE), because Migrate() re-runs on cold starts (CLAUDE.md
-- rule 4).
--
-- Cap design: attempt_no is unique per (user_id, course_id) and a BEFORE
-- INSERT trigger rejects attempt_no > 5 + quiz_attempt_grants.extra.  With
-- no grant row the cap is exactly 5, whatever the application does.  The
-- trigger takes a per-(user, course) advisory lock so concurrent starts
-- serialise; the unique constraint is the final backstop.

ALTER TABLE quiz_questions ADD COLUMN IF NOT EXISTS set_no SMALLINT;
ALTER TABLE quiz_questions ADD COLUMN IF NOT EXISTS module TEXT NOT NULL DEFAULT '';

ALTER TABLE quiz_attempts ADD COLUMN IF NOT EXISTS attempt_no INT;
ALTER TABLE quiz_attempts ADD COLUMN IF NOT EXISTS set_no SMALLINT;
ALTER TABLE quiz_attempts ADD COLUMN IF NOT EXISTS question_ids BIGINT[] NOT NULL DEFAULT '{}';
ALTER TABLE quiz_attempts ADD COLUMN IF NOT EXISTS submitted_at TIMESTAMPTZ;

-- Attempts recorded before this migration were all complete.
UPDATE quiz_attempts SET submitted_at = created_at
 WHERE submitted_at IS NULL AND cardinality(question_ids) = 0;

-- Number legacy visitor attempts (admin previews keep NULL: they are not capped).
UPDATE quiz_attempts q SET attempt_no = n.rn
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id, course_id ORDER BY id) AS rn
          FROM quiz_attempts WHERE user_id IS NOT NULL AND attempt_no IS NULL) n
 WHERE q.id = n.id AND q.attempt_no IS NULL AND NOT EXISTS (
        SELECT 1 FROM quiz_attempts x
         WHERE x.user_id = q.user_id AND x.course_id = q.course_id AND x.attempt_no = n.rn);

CREATE TABLE IF NOT EXISTS quiz_attempt_grants (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id  BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    extra      INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, course_id)
);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'quiz_questions_set_no_chk') THEN
        ALTER TABLE quiz_questions ADD CONSTRAINT quiz_questions_set_no_chk
            CHECK (set_no IS NULL OR set_no BETWEEN 1 AND 5);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'quiz_attempts_set_no_chk') THEN
        ALTER TABLE quiz_attempts ADD CONSTRAINT quiz_attempts_set_no_chk
            CHECK (set_no IS NULL OR set_no BETWEEN 1 AND 5);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'quiz_attempts_attempt_no_chk') THEN
        ALTER TABLE quiz_attempts ADD CONSTRAINT quiz_attempts_attempt_no_chk
            CHECK (attempt_no IS NULL OR attempt_no >= 1);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'quiz_attempts_user_course_no_uk') THEN
        ALTER TABLE quiz_attempts ADD CONSTRAINT quiz_attempts_user_course_no_uk
            UNIQUE (user_id, course_id, attempt_no);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'quiz_attempt_grants_extra_chk') THEN
        ALTER TABLE quiz_attempt_grants ADD CONSTRAINT quiz_attempt_grants_extra_chk
            CHECK (extra BETWEEN 0 AND 50);
    END IF;
END $$;

CREATE OR REPLACE FUNCTION quiz_attempt_cap_guard() RETURNS trigger AS $$
DECLARE
    granted INT;
BEGIN
    IF NEW.user_id IS NULL OR NEW.attempt_no IS NULL THEN
        RETURN NEW;
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended('quiz_attempt:' || NEW.user_id || ':' || NEW.course_id, 0));
    SELECT g.extra INTO granted FROM quiz_attempt_grants g
     WHERE g.user_id = NEW.user_id AND g.course_id = NEW.course_id;
    IF NEW.attempt_no > 5 + COALESCE(granted, 0) THEN
        RAISE EXCEPTION 'quiz attempt limit reached' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS quiz_attempt_cap_trg ON quiz_attempts;
CREATE TRIGGER quiz_attempt_cap_trg BEFORE INSERT ON quiz_attempts
    FOR EACH ROW EXECUTE FUNCTION quiz_attempt_cap_guard();

CREATE INDEX IF NOT EXISTS idx_quiz_questions_course_set ON quiz_questions(course_id, set_no);
CREATE INDEX IF NOT EXISTS idx_quiz_attempts_open ON quiz_attempts(course_id, user_id) WHERE submitted_at IS NULL;
