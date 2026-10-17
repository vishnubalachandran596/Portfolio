-- The server-side code runner (migration 081) was dropped: JDoodle puts a
-- reCAPTCHA in front of runs from a headless browser. Idempotent.
DROP TABLE IF EXISTS code_runs;
