-- Migration 005 DOWN: Revert batch-based scanning
-- WARNING: This migration cannot fully restore root_domain_id links
-- LOC records will need to be re-associated manually after rollback

-- Drop new tables
DROP TABLE IF EXISTS scan_batches CASCADE;
DROP TABLE IF EXISTS domain_files CASCADE;

-- Recreate old tables
CREATE TABLE root_domains (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain              TEXT NOT NULL UNIQUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_scanned_at     TIMESTAMPTZ,
    subdomains_scanned  BIGINT NOT NULL DEFAULT 0,
    queued_at           TIMESTAMPTZ
);

CREATE INDEX idx_root_domains_last_scanned ON root_domains(last_scanned_at NULLS FIRST);

CREATE TABLE active_scans (
    root_domain_id  UUID PRIMARY KEY REFERENCES root_domains(id) ON DELETE CASCADE,
    client_id       UUID NOT NULL REFERENCES scanner_clients(id) ON DELETE CASCADE,
    session_id      TEXT,
    assigned_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_active_scans_client ON active_scans(client_id);
CREATE INDEX idx_active_scans_assigned ON active_scans(assigned_at);

-- Insert root domains from loc_records.root_domain
INSERT INTO root_domains (domain)
SELECT DISTINCT root_domain FROM loc_records
ON CONFLICT (domain) DO NOTHING;

-- Add back root_domain_id column
ALTER TABLE loc_records ADD COLUMN root_domain_id UUID;

-- Populate root_domain_id from root_domain text
UPDATE loc_records l
SET root_domain_id = rd.id
FROM root_domains rd
WHERE rd.domain = l.root_domain;

-- Add FK constraint
ALTER TABLE loc_records
ADD CONSTRAINT loc_records_root_domain_id_fkey
FOREIGN KEY (root_domain_id) REFERENCES root_domains(id) ON DELETE CASCADE;

-- Make NOT NULL
ALTER TABLE loc_records ALTER COLUMN root_domain_id SET NOT NULL;

-- Drop root_domain text column
DROP INDEX IF EXISTS idx_loc_records_root_domain;
ALTER TABLE loc_records DROP COLUMN root_domain;

-- Recreate old index
CREATE INDEX idx_loc_records_root_domain ON loc_records(root_domain_id);
