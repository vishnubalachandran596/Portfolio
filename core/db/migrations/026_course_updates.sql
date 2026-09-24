-- A course update is recorded as a single row here — the "db entry" that
-- marks a course as updated and what changed. A trigger fans it out to
-- every paid buyer as an admin_messages row (the same table and mechanism
-- that already powers the notification bell and /account#messages), so
-- notifying buyers never needs more than this one insert.
CREATE TABLE IF NOT EXISTS course_updates (
    id BIGSERIAL PRIMARY KEY,
    course_id BIGINT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    note TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_course_updates_course ON course_updates(course_id);

CREATE OR REPLACE FUNCTION notify_course_update() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO admin_messages (user_id, body)
    SELECT DISTINCT p.user_id,
        '"' || c.title || '" was updated: ' || NEW.note ||
        COALESCE((SELECT E'\n\nFeel free to reach out to me at ' || email FROM profile WHERE email <> '' LIMIT 1), '')
    FROM purchases p
    JOIN courses c ON c.id = p.course_id
    WHERE p.course_id = NEW.course_id
      AND p.status = 'paid'
      AND p.user_id IS NOT NULL;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_course_updates_notify ON course_updates;
CREATE TRIGGER trg_course_updates_notify
    AFTER INSERT ON course_updates
    FOR EACH ROW EXECUTE FUNCTION notify_course_update();
