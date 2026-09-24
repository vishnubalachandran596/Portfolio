-- This app talks to Postgres directly (not via PostgREST/Supabase client
-- libraries), so the auto-exposed REST API over these tables is unused
-- attack surface on a Supabase-hosted database. Enable RLS with zero
-- policies on every table: PostgREST's anon/authenticated roles are then
-- denied by default. The table owner (this app's own DATABASE_URL role)
-- and superusers always bypass RLS, so application behavior is unaffected
-- on Supabase or on a plain self-hosted Postgres.
ALTER TABLE admins ENABLE ROW LEVEL SECURITY;
ALTER TABLE profile ENABLE ROW LEVEL SECURITY;
ALTER TABLE skill_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE experiences ENABLE ROW LEVEL SECURITY;
ALTER TABLE education ENABLE ROW LEVEL SECURITY;
ALTER TABLE home_features ENABLE ROW LEVEL SECURITY;
ALTER TABLE courses ENABLE ROW LEVEL SECURITY;
ALTER TABLE availability_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE bookings ENABLE ROW LEVEL SECURITY;
ALTER TABLE schema_migrations ENABLE ROW LEVEL SECURITY;
