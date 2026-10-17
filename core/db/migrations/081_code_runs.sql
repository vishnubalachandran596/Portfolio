-- One row per run sent to the server-side code runner (api/run.js). It backs
-- the per-learner and daily limits that protect the JDoodle plan, and shows
-- how much the editor is used. Idempotent: IF NOT EXISTS throughout.
CREATE TABLE IF NOT EXISTS code_runs (
    id         BIGSERIAL PRIMARY KEY,
    course_id  BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    actor      TEXT   NOT NULL,           -- "p<purchase id>" for a buyer, "admin" for an admin
    lang       TEXT   NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS code_runs_created_idx ON code_runs (created_at);
CREATE INDEX IF NOT EXISTS code_runs_actor_idx ON code_runs (actor, created_at);
