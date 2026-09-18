-- Corrects a misattribution caught during verification: the article's
-- byline author is Gergely Orosz (The Pragmatic Engineer), drawing on
-- Nicky Wrightson's account — not Wrightson himself as the writer.
UPDATE articles
SET summary = 'Gergely Orosz, writing for The Pragmatic Engineer, drawing on Nicky Wrightson''s (a former principal engineer) account of what actually changes when you move from senior engineer to staff+: less about writing more code, more about picking the right problems, being trusted with ambiguity, and, as the title says, becoming a role model whether you signed up for that or not. I recommend this to anyone plotting a technical (not management) career path who''s tired of vague leveling-guide language and wants the real texture of the job. Refreshingly specific about the frustrating parts too, not just the flattering ones.'
WHERE slug = 'what-is-a-staff-engineer';
