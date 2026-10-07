-- Short one-liner for the catalog grid card and the product page's hero,
-- separate from the long-form description -- mirrors courses.summary vs
-- courses.description. Without this, software_list.html/software_detail.html
-- had only software.description to work with in both a short-card and a
-- full-page context, which duplicated the whole description across the two.
ALTER TABLE software ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';
