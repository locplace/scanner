-- Remove coordinate validation constraint
ALTER TABLE loc_records DROP CONSTRAINT IF EXISTS valid_coordinates;
