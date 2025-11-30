-- Rollback Migration 003: Remove session_id columns

DROP INDEX IF EXISTS idx_active_scans_session;
ALTER TABLE active_scans DROP COLUMN IF EXISTS session_id;
ALTER TABLE scanner_clients DROP COLUMN IF EXISTS session_id;
