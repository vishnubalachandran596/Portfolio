-- Consultancy is billed per hour, so every availability slot should be a
-- full hour, not the original 30-minute default.
UPDATE availability_rules SET slot_minutes = 60;
