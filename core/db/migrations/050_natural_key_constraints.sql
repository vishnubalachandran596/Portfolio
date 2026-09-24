-- Natural-key uniqueness for every content table that is addressed by one.
--
-- skill_groups, experiences, education, home_features, availability_rules
-- and quiz_questions all have a SERIAL surrogate PK and no unique constraint
-- on the key a human (or a snapshot file) actually identifies the row by.
-- Without this, a re-run of a WHERE-NOT-EXISTS-unguarded insert duplicates
-- every row. That already happened in production to quiz_questions.
--
-- Order matters: the DELETEs must run before the ADD CONSTRAINTs, or the
-- constraint fails against existing duplicate data and takes the site down
-- on cold start.
--
-- articles.slug is already unique via idx_articles_slug (migration 014).
-- UNIQUE USING INDEX promotes that index in place — no rebuild.
--
-- Idempotency (rule 4): the DELETEs are self-limiting (nothing left to
-- delete on a second pass) and ADD CONSTRAINT is wrapped in the
-- pg_constraint guard, since Postgres has no ADD CONSTRAINT IF NOT EXISTS.

DELETE FROM quiz_questions q USING quiz_questions dup
 WHERE q.course_id = dup.course_id AND q.question = dup.question AND q.id > dup.id;

DELETE FROM skill_groups s USING skill_groups dup
 WHERE s.category = dup.category AND s.id > dup.id;

DELETE FROM experiences e USING experiences dup
 WHERE e.company = dup.company AND e.role = dup.role
   AND e.start_date = dup.start_date AND e.id > dup.id;

DELETE FROM education e USING education dup
 WHERE e.institution = dup.institution AND e.degree = dup.degree
   AND e.field = dup.field AND e.id > dup.id;

DELETE FROM home_features h USING home_features dup
 WHERE h.title = dup.title AND h.id > dup.id;

DELETE FROM availability_rules a USING availability_rules dup
 WHERE a.weekday = dup.weekday AND a.start_time = dup.start_time
   AND a.end_time = dup.end_time AND a.id > dup.id;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'skill_groups_category_uk') THEN
        ALTER TABLE skill_groups ADD CONSTRAINT skill_groups_category_uk UNIQUE (category);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'experiences_company_role_start_uk') THEN
        ALTER TABLE experiences ADD CONSTRAINT experiences_company_role_start_uk
            UNIQUE (company, role, start_date);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'education_institution_degree_field_uk') THEN
        ALTER TABLE education ADD CONSTRAINT education_institution_degree_field_uk
            UNIQUE (institution, degree, field);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'home_features_title_uk') THEN
        ALTER TABLE home_features ADD CONSTRAINT home_features_title_uk UNIQUE (title);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'availability_rules_slot_uk') THEN
        ALTER TABLE availability_rules ADD CONSTRAINT availability_rules_slot_uk
            UNIQUE (weekday, start_time, end_time);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'quiz_questions_course_question_uk') THEN
        ALTER TABLE quiz_questions ADD CONSTRAINT quiz_questions_course_question_uk
            UNIQUE (course_id, question);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'articles_slug_uk') THEN
        ALTER TABLE articles ADD CONSTRAINT articles_slug_uk UNIQUE USING INDEX idx_articles_slug;
    END IF;
END $$;
