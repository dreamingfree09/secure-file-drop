-- Reverting a live managed-link ledger could silently re-authorise issued
-- tokens or destroy revocation decisions. Require a separately reviewed
-- paired checkpoint/restore instead of an automatic destructive downgrade.
DO $$ BEGIN RAISE EXCEPTION 'managed share schema downgrade is unsupported'; END $$;
