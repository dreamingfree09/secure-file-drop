-- Resumable reservations may still own private multipart objects. An operator
-- must reconcile and restore an independently verified checkpoint to roll back.
DO $$ BEGIN RAISE EXCEPTION 'resumable schema downgrade is unsupported'; END $$;
