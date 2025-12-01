-- Rollback: Revert scanner_id back to TEXT

DROP INDEX IF EXISTS idx_batches_scanner;

ALTER TABLE scan_batches DROP CONSTRAINT IF EXISTS fk_scan_batches_scanner;

ALTER TABLE scan_batches
    ALTER COLUMN scanner_id TYPE TEXT USING scanner_id::text;
