package wameow

// The upstream sqlstore creates tables outside our migrations. Keep its private
// protocol/session tables inaccessible through Supabase's public Data API,
// including tables added by future library upgrades. The Go database owner
// continues to access them directly.
const protectStoreTables = `DO $$
DECLARE r record;
BEGIN
 FOR r IN SELECT schemaname,tablename FROM pg_tables
 WHERE schemaname='public' AND left(tablename,10)='whatsmeow_'
 LOOP
  EXECUTE format('ALTER TABLE %I.%I ENABLE ROW LEVEL SECURITY',r.schemaname,r.tablename);
  EXECUTE format('REVOKE ALL ON TABLE %I.%I FROM PUBLIC',r.schemaname,r.tablename);
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN
   EXECUTE format('REVOKE ALL ON TABLE %I.%I FROM anon',r.schemaname,r.tablename);
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN
   EXECUTE format('REVOKE ALL ON TABLE %I.%I FROM authenticated',r.schemaname,r.tablename);
  END IF;
 END LOOP;
END $$;`
