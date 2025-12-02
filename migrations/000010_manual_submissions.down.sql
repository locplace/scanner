-- Delete any batches associated with manual submissions first (FK constraint)
DELETE FROM scan_batches WHERE file_id = (
    SELECT id FROM domain_files WHERE filename = '__manual_submissions__'
);

-- Delete the pseudo file
DELETE FROM domain_files WHERE filename = '__manual_submissions__';
