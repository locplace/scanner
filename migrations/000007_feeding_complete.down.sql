DROP INDEX IF EXISTS idx_domain_files_needs_feeding;
ALTER TABLE domain_files DROP COLUMN IF EXISTS feeding_complete;
