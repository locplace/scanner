-- Migration 006: Fix scanner_id type in scan_batches
-- Change from TEXT to UUID to match scanner_clients.id

-- Step 1: Drop indexes that reference scanner_id (if any exist implicitly)
-- The partial indexes don't reference scanner_id directly, so we're ok

-- Step 2: Alter the column type (NULL values will remain NULL)
ALTER TABLE scan_batches
    ALTER COLUMN scanner_id TYPE UUID USING scanner_id::uuid;

-- Step 3: Add foreign key constraint for referential integrity
ALTER TABLE scan_batches
    ADD CONSTRAINT fk_scan_batches_scanner
    FOREIGN KEY (scanner_id) REFERENCES scanner_clients(id) ON DELETE SET NULL;

-- Step 4: Add index for the foreign key
CREATE INDEX idx_batches_scanner ON scan_batches(scanner_id) WHERE scanner_id IS NOT NULL;
