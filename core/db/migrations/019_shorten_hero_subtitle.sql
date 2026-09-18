-- The hero subtitle doubles as the Home/About meta description, so it
-- needs to earn its keep in one short line — the trailing clause about
-- architecture/system design/leadership is now covered by the skills
-- achievements section right below the hero, so it's redundant here.
UPDATE profile SET hero_subtitle =
'Technical Consultant and Java technical expert with 8+ years building resilient banking and fintech platforms.'
WHERE id = 1;
