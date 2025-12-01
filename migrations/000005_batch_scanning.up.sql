-- Migration 005: Batch-based scanning with domains project
-- Replaces root domain scanning with direct FQDN batch scanning

-- Step 1: Add root_domain column to loc_records
ALTER TABLE loc_records ADD COLUMN root_domain TEXT;

-- Step 2: Populate root_domain from existing join
UPDATE loc_records l
SET root_domain = rd.domain
FROM root_domains rd
WHERE rd.id = l.root_domain_id;

-- Step 3: Make root_domain NOT NULL (all records should now have it)
-- Handle case where there might be orphaned records
DELETE FROM loc_records WHERE root_domain IS NULL;
ALTER TABLE loc_records ALTER COLUMN root_domain SET NOT NULL;

-- Step 4: Drop old FK constraint and column
ALTER TABLE loc_records DROP CONSTRAINT loc_records_root_domain_id_fkey;
ALTER TABLE loc_records DROP COLUMN root_domain_id;

-- Step 5: Drop old index that referenced root_domain_id
DROP INDEX IF EXISTS idx_loc_records_root_domain;

-- Step 6: Add new index on root_domain text
CREATE INDEX idx_loc_records_root_domain ON loc_records(root_domain);

-- Step 7: Drop old tables (order matters for FK constraints)
DROP TABLE IF EXISTS active_scans CASCADE;
DROP TABLE IF EXISTS domain_sets CASCADE;
DROP TABLE IF EXISTS root_domains CASCADE;

-- Step 8: Create domain_files table (tracks .xz files from domains project)
CREATE TABLE domain_files (
    id                  SERIAL PRIMARY KEY,
    filename            TEXT UNIQUE NOT NULL,
    url                 TEXT NOT NULL,
    size_bytes          BIGINT,
    processed_lines     BIGINT NOT NULL DEFAULT 0,
    batches_created     INT NOT NULL DEFAULT 0,
    batches_completed   INT NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'pending',
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,

    CONSTRAINT valid_file_status CHECK (status IN ('pending', 'processing', 'complete'))
);

CREATE INDEX idx_domain_files_status ON domain_files(status);

-- Step 9: Create scan_batches table (work queue)
CREATE TABLE scan_batches (
    id              BIGSERIAL PRIMARY KEY,
    file_id         INT NOT NULL REFERENCES domain_files(id) ON DELETE CASCADE,
    line_start      BIGINT NOT NULL,
    line_end        BIGINT NOT NULL,
    domains         TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    assigned_at     TIMESTAMPTZ,
    scanner_id      TEXT,

    CONSTRAINT valid_batch_status CHECK (status IN ('pending', 'in_flight'))
);

-- Partial indexes for efficient queue operations
CREATE INDEX idx_batches_pending ON scan_batches(id) WHERE status = 'pending';
CREATE INDEX idx_batches_stale ON scan_batches(assigned_at) WHERE status = 'in_flight';
CREATE INDEX idx_batches_file ON scan_batches(file_id);
