-- Destroying registration authority or provenance is not a safe rollback.
DO $$ BEGIN RAISE EXCEPTION 'registration schema downgrade requires separately reviewed paired recovery'; END $$;
