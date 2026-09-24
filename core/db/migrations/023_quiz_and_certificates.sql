CREATE TABLE IF NOT EXISTS quiz_questions (
    id BIGSERIAL PRIMARY KEY,
    course_id BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    question TEXT NOT NULL,
    option_a TEXT NOT NULL,
    option_b TEXT NOT NULL,
    option_c TEXT NOT NULL,
    option_d TEXT NOT NULL,
    correct_option CHAR(1) NOT NULL CHECK (correct_option IN ('A','B','C','D')),
    sort_order INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS quiz_attempts (
    id BIGSERIAL PRIMARY KEY,
    course_id BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    total_questions INT NOT NULL,
    correct_answers INT NOT NULL,
    score_percent INT NOT NULL,
    passed BOOLEAN NOT NULL,
    certificate_code TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_quiz_attempts_course_email ON quiz_attempts(course_id, email);
CREATE INDEX IF NOT EXISTS idx_quiz_questions_course ON quiz_questions(course_id);
