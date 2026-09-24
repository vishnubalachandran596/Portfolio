-- Article HTML lives in its own private bucket, `articles`, not inside
-- course-content. Object keys stay articles/{slug}.html. The bucket and
-- policy exist only on Supabase; this block no-ops on a local Postgres
-- that has no storage schema. The same dedicated login that reads
-- course-content can read and write this bucket. Re-running is a no-op:
-- the insert conflicts on id, and the policy is created only when missing.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'storage') THEN
        INSERT INTO storage.buckets (id, name, public, file_size_limit)
        VALUES ('articles', 'articles', false, 8388608)
        ON CONFLICT (id) DO NOTHING;

        IF NOT EXISTS (
            SELECT 1 FROM pg_policies
            WHERE schemaname = 'storage'
              AND tablename = 'objects'
              AND policyname = 'articles_reader'
        ) THEN
            CREATE POLICY articles_reader ON storage.objects
                FOR ALL
                TO authenticated
                USING (
                    bucket_id = 'articles'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                )
                WITH CHECK (
                    bucket_id = 'articles'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                );
        END IF;
    END IF;
END $$;
