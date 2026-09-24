-- Consultancy is billed per hour, so every availability slot should be a
-- full hour, not the original 30-minute default.
ALTER TABLE availability_rules ALTER COLUMN slot_minutes SET DEFAULT 60;
