-- Retry throttle for the preview builder (handlers.buildCoursePreview).
-- The course page rebuilds outline + preview_ready when either is missing;
-- preview_checked_at records the last attempt so a lesson with no
-- previewable section is not re-read from Storage on every view. Idempotent.
ALTER TABLE courses ADD COLUMN IF NOT EXISTS preview_checked_at TIMESTAMPTZ;
