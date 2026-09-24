-- One PDF resume per role. The file itself lives in the private Storage
-- bucket `resumes` (object key resumes/{role_key}.pdf). This table only
-- records the key, the label shown in the download select, and the path.
-- No resume row is inserted here: the PDF is content, and content is not
-- part of a migration.
--
-- RLS with zero policies closes the PostgREST door. The app connects as
-- the table owner and bypasses RLS. ENABLE ROW LEVEL SECURITY is a no-op
-- when it is already on.
--
-- The bucket and policy exist only on Supabase; the block no-ops on a
-- local Postgres that has no storage schema. The same dedicated login
-- that reads course-content and articles can read and write this bucket.
-- Re-running is a no-op: the insert conflicts on id, and the policy is
-- created only when missing.

CREATE TABLE IF NOT EXISTS role_resumes (
    role_key    TEXT PRIMARY KEY,
    label       TEXT NOT NULL DEFAULT '',
    object_path TEXT NOT NULL DEFAULT '',
    sort_order  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE role_resumes ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'storage') THEN
        INSERT INTO storage.buckets (id, name, public, file_size_limit)
        VALUES ('resumes', 'resumes', false, 8388608)
        ON CONFLICT (id) DO NOTHING;

        IF NOT EXISTS (
            SELECT 1 FROM pg_policies
            WHERE schemaname = 'storage'
              AND tablename = 'objects'
              AND policyname = 'resumes_reader'
        ) THEN
            CREATE POLICY resumes_reader ON storage.objects
                FOR ALL
                TO authenticated
                USING (
                    bucket_id = 'resumes'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                )
                WITH CHECK (
                    bucket_id = 'resumes'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                );
        END IF;
    END IF;
END $$;
