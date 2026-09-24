-- Paid lesson HTML lives in a private Supabase Storage bucket, not in
-- the repo and not in courses.content_html. content_object is the path
-- inside that bucket (courses/{slug}.html). The learn handler streams
-- the object after the purchase check.
--
-- The bucket and its policy exist only on Supabase (the storage schema
-- is not part of a local Postgres). The DO block no-ops there. The
-- policy allows one dedicated login, course-content@vishnubalachandran.com, and
-- nobody else. That password is an environment variable, never a
-- migration. The service-role key bypasses this policy; the anon key
-- does not.

ALTER TABLE courses ADD COLUMN IF NOT EXISTS content_object TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'storage') THEN
        INSERT INTO storage.buckets (id, name, public, file_size_limit)
        VALUES ('course-content', 'course-content', false, 8388608)
        ON CONFLICT (id) DO NOTHING;

        IF NOT EXISTS (
            SELECT 1 FROM pg_policies
            WHERE schemaname = 'storage'
              AND tablename = 'objects'
              AND policyname = 'course_content_reader'
        ) THEN
            CREATE POLICY course_content_reader ON storage.objects
                FOR ALL
                TO authenticated
                USING (
                    bucket_id = 'course-content'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                )
                WITH CHECK (
                    bucket_id = 'course-content'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                );
        END IF;
    END IF;
END $$;
