-- Add feeding_complete flag to track when all lines have been read from a file
-- This allows the feeder to move on to the next file while batches are still being processed
ALTER TABLE domain_files
    ADD COLUMN feeding_complete BOOLEAN NOT NULL DEFAULT FALSE;

-- Index for efficient querying of files that need feeding
CREATE INDEX idx_domain_files_needs_feeding
    ON domain_files(status, feeding_complete)
    WHERE status IN ('pending', 'processing') AND feeding_complete = false;
