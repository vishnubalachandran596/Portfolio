// admin_content.go covers the admin CRUD for About Me content that only
// had migration-file seeding before: skill categories and work
// experience entries. Education stays migration-only for now — it
// changes far less often than skills/experience.
package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// --- Skills --------------------------------------------------------------

type AdminSkillsListData struct {
	BaseData
	Skills []models.SkillGroup
}

func (a *App) AdminSkillsList(w http.ResponseWriter, r *http.Request) {
	skills, err := a.Skills.List(r.Context())
	if err != nil {
		a.serverError(w, "load skills", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_skills_list", AdminSkillsListData{
		BaseData: a.base(r, "Skills — Admin", "", "admin"),
		Skills:   skills,
	})
}

type AdminSkillFormData struct {
	BaseData
	Skill models.SkillGroup
	IsNew bool
	Error string
}

func (a *App) AdminSkillNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_skill_form", AdminSkillFormData{
		BaseData: a.base(r, "New Skill — Admin", "", "admin"),
		IsNew:    true,
	})
}

func (a *App) AdminSkillEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	skill, err := a.Skills.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load skill", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_skill_form", AdminSkillFormData{
		BaseData: a.base(r, "Edit Skill — Admin", "", "admin"),
		Skill:    skill,
	})
}

func (a *App) AdminSkillSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var id int64
	if idStr := r.PathValue("id"); idStr != "" {
		var err error
		id, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			a.NotFound(w, r)
			return
		}
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	skill := models.SkillGroup{
		ID:          id,
		Category:    strings.TrimSpace(r.FormValue("category")),
		Level:       strings.TrimSpace(r.FormValue("level")),
		Achievement: strings.TrimSpace(r.FormValue("achievement")),
		Items:       parseTags(r.FormValue("items")),
		SortOrder:   sortOrder,
	}
	if skill.Category == "" || len(skill.Items) == 0 {
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_skill_form", AdminSkillFormData{
			BaseData: a.base(r, "Skill — Admin", "", "admin"),
			Skill:    skill,
			IsNew:    id == 0,
			Error:    "Category and at least one skill item are required.",
		})
		return
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.Skills.Create(ctx, skill)
	} else {
		err = a.Skills.Update(ctx, skill)
	}
	if err != nil {
		a.serverError(w, "save skill", err)
		return
	}
	http.Redirect(w, r, "/admin/skills", http.StatusSeeOther)
}

func (a *App) AdminSkillDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Skills.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete skill", err)
		return
	}
	http.Redirect(w, r, "/admin/skills", http.StatusSeeOther)
}

// --- Experience ------------------------------------------------------------

type AdminExperienceListData struct {
	BaseData
	Experience []models.Experience
}

func (a *App) AdminExperienceList(w http.ResponseWriter, r *http.Request) {
	exp, err := a.Experience.List(r.Context())
	if err != nil {
		a.serverError(w, "load experience", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_experience_list", AdminExperienceListData{
		BaseData:   a.base(r, "Experience — Admin", "", "admin"),
		Experience: exp,
	})
}

type AdminExperienceFormData struct {
	BaseData
	Experience models.Experience
	IsNew      bool
	Error      string
}

func (a *App) AdminExperienceNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_experience_form", AdminExperienceFormData{
		BaseData: a.base(r, "New Experience — Admin", "", "admin"),
		IsNew:    true,
	})
}

func (a *App) AdminExperienceEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	exp, err := a.Experience.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load experience", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_experience_form", AdminExperienceFormData{
		BaseData:   a.base(r, "Edit Experience — Admin", "", "admin"),
		Experience: exp,
	})
}

func (a *App) AdminExperienceSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var id int64
	if idStr := r.PathValue("id"); idStr != "" {
		var err error
		id, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			a.NotFound(w, r)
			return
		}
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	var bullets []string
	for _, line := range strings.Split(r.FormValue("bullets"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			bullets = append(bullets, line)
		}
	}
	exp := models.Experience{
		ID:        id,
		Company:   strings.TrimSpace(r.FormValue("company")),
		Role:      strings.TrimSpace(r.FormValue("role")),
		Location:  strings.TrimSpace(r.FormValue("location")),
		StartDate: strings.TrimSpace(r.FormValue("start_date")),
		EndDate:   strings.TrimSpace(r.FormValue("end_date")),
		Bullets:   bullets,
		SortOrder: sortOrder,
	}
	if exp.Company == "" || exp.Role == "" {
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_experience_form", AdminExperienceFormData{
			BaseData:   a.base(r, "Experience — Admin", "", "admin"),
			Experience: exp,
			IsNew:      id == 0,
			Error:      "Company and role are required.",
		})
		return
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.Experience.Create(ctx, exp)
	} else {
		err = a.Experience.Update(ctx, exp)
	}
	if err != nil {
		a.serverError(w, "save experience", err)
		return
	}
	http.Redirect(w, r, "/admin/experience", http.StatusSeeOther)
}

func (a *App) AdminExperienceDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Experience.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete experience", err)
		return
	}
	http.Redirect(w, r, "/admin/experience", http.StatusSeeOther)
}

type AdminTestimonialsListData struct {
	BaseData
	Testimonials []models.Testimonial
}

func (a *App) AdminTestimonialsList(w http.ResponseWriter, r *http.Request) {
	list, err := a.Testimonials.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load testimonials", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_testimonials_list", AdminTestimonialsListData{
		BaseData:     a.base(r, "Testimonials — Admin", "", "admin"),
		Testimonials: list,
	})
}

type AdminTestimonialFormData struct {
	BaseData
	Testimonial models.Testimonial
	IsNew       bool
	Error       string
}

func (a *App) AdminTestimonialNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_testimonial_form", AdminTestimonialFormData{
		BaseData:    a.base(r, "New Testimonial — Admin", "", "admin"),
		IsNew:       true,
		Testimonial: models.Testimonial{IsPublished: true},
	})
}

func (a *App) AdminTestimonialEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	t, err := a.Testimonials.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load testimonial", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_testimonial_form", AdminTestimonialFormData{
		BaseData:    a.base(r, "Edit Testimonial — Admin", "", "admin"),
		Testimonial: t,
	})
}

func (a *App) AdminTestimonialSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var id int64
	if idStr := r.PathValue("id"); idStr != "" {
		var err error
		id, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			a.NotFound(w, r)
			return
		}
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	t := models.Testimonial{
		ID:          id,
		AuthorName:  strings.TrimSpace(r.FormValue("author_name")),
		AuthorTitle: strings.TrimSpace(r.FormValue("author_title")),
		Body:        strings.TrimSpace(r.FormValue("body")),
		IsPublished: r.FormValue("is_published") == "on",
		SortOrder:   sortOrder,
	}
	if t.AuthorName == "" || t.Body == "" {
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_testimonial_form", AdminTestimonialFormData{
			BaseData:    a.base(r, "Testimonial — Admin", "", "admin"),
			Testimonial: t,
			IsNew:       id == 0,
			Error:       "Name and quote are required.",
		})
		return
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.Testimonials.Create(ctx, t)
	} else {
		err = a.Testimonials.Update(ctx, t)
	}
	if err != nil {
		a.serverError(w, "save testimonial", err)
		return
	}
	http.Redirect(w, r, "/admin/testimonials", http.StatusSeeOther)
}

// AdminTestimonialApprove publishes a visitor-submitted testimonial with a
// single click, without needing to open the full edit form.
func (a *App) AdminTestimonialApprove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Testimonials.Approve(r.Context(), id); err != nil {
		a.serverError(w, "approve testimonial", err)
		return
	}
	http.Redirect(w, r, "/admin/testimonials", http.StatusSeeOther)
}

func (a *App) AdminTestimonialDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Testimonials.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete testimonial", err)
		return
	}
	http.Redirect(w, r, "/admin/testimonials", http.StatusSeeOther)
}
