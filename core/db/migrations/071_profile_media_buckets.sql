-- Profile photo and site logo move out of the repo entirely and into
-- their own private Storage buckets, uploaded via the admin console
-- (core/handlers/admin.go) rather than typed in as a URL. photo_url
-- held a path into the repo's own web/static/img/ -- the whole point
-- of this migration is that the repo stops holding any image at all,
-- so that column is replaced rather than kept alongside the new one.
--
-- photo_object/logo_object store the Storage object path (e.g.
-- "profile/photo.jpg"), mirroring role_resumes.object_path -- empty
-- means nothing uploaded yet. The app always serves these through its
-- own GET /media/profile-photo and /media/logo routes (never a bucket
-- URL directly), the same streaming pattern already used for resumes,
-- so swapping the image takes effect immediately with no risk of the
-- browser/CDN caching a stale file at a reused URL (see the incident
-- this project already hit once with web/static/img/vishnu-photo.jpg).
--
-- The buckets and their policy exist only on Supabase; the DO block
-- no-ops on a local Postgres with no storage schema. Reuses the same
-- dedicated reader login (course-content@vishnubalachandran.com)
-- already trusted for course-content/articles/resumes -- one login,
-- scoped per-bucket by policy, not a new credential per bucket.

ALTER TABLE profile DROP COLUMN IF EXISTS photo_url;
ALTER TABLE profile ADD COLUMN IF NOT EXISTS photo_object TEXT NOT NULL DEFAULT '';
ALTER TABLE profile ADD COLUMN IF NOT EXISTS logo_object TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'storage') THEN
        INSERT INTO storage.buckets (id, name, public, file_size_limit)
        VALUES ('profile-photo', 'profile-photo', false, 5242880)
        ON CONFLICT (id) DO NOTHING;

        INSERT INTO storage.buckets (id, name, public, file_size_limit)
        VALUES ('site-logo', 'site-logo', false, 5242880)
        ON CONFLICT (id) DO NOTHING;

        IF NOT EXISTS (
            SELECT 1 FROM pg_policies
            WHERE schemaname = 'storage'
              AND tablename = 'objects'
              AND policyname = 'profile_photo_reader'
        ) THEN
            CREATE POLICY profile_photo_reader ON storage.objects
                FOR ALL
                TO authenticated
                USING (
                    bucket_id = 'profile-photo'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                )
                WITH CHECK (
                    bucket_id = 'profile-photo'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                );
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_policies
            WHERE schemaname = 'storage'
              AND tablename = 'objects'
              AND policyname = 'site_logo_reader'
        ) THEN
            CREATE POLICY site_logo_reader ON storage.objects
                FOR ALL
                TO authenticated
                USING (
                    bucket_id = 'site-logo'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                )
                WITH CHECK (
                    bucket_id = 'site-logo'
                    AND (auth.jwt() ->> 'email') = 'course-content@vishnubalachandran.com'
                );
        END IF;
    END IF;
END $$;
