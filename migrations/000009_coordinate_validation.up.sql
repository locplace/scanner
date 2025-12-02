-- Add constraint to ensure coordinates are within valid bounds
-- Latitude: -90 to 90, Longitude: -180 to 180

-- First, delete any existing invalid records
DELETE FROM loc_records
WHERE latitude < -90 OR latitude > 90
   OR longitude < -180 OR longitude > 180;

-- Then add the constraint
ALTER TABLE loc_records
ADD CONSTRAINT valid_coordinates CHECK (
    latitude >= -90 AND latitude <= 90
    AND longitude >= -180 AND longitude <= 180
);
