-- Rollback migration 008: Remove scanner sessions

DROP INDEX IF EXISTS idx_batches_session;
ALTER TABLE scan_batches DROP COLUMN IF EXISTS session_id;

DROP TABLE IF EXISTS scanner_sessions;
