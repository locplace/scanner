-- Create a pseudo file entry for manual domain submissions.
-- This allows manual batches to use the existing file/batch tracking system.
-- The file is marked complete so the feeder ignores it.
INSERT INTO domain_files (filename, url, size_bytes, status, feeding_complete)
VALUES ('__manual_submissions__', '', 0, 'complete', true);
