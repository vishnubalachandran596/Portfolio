-- Each skill category gets a one-line "what I achieve with this" so the
-- About/Home skills grid reads as real, grounded accomplishments rather
-- than a bare tag list.
ALTER TABLE skill_groups ADD COLUMN IF NOT EXISTS achievement TEXT NOT NULL DEFAULT '';
