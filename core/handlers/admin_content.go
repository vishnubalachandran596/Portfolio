// admin_content.go covers the admin CRUD for About Me content that only
// had migration-file seeding before: skill categories and work
// experience entries. Education stays migration-only for now — it
// changes far less often than skills/experience.
package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/storage"
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
	Resume     models.RoleResume
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
	resume, err := a.resumeForExperience(r.Context(), exp)
	if err != nil {
		a.serverError(w, "load resume", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_experience_form", AdminExperienceFormData{
		BaseData:   a.base(r, "Edit Experience — Admin", "", "admin"),
		Experience: exp,
		Resume:     resume,
	})
}

func (a *App) AdminExperienceSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxCourseUploadBytes); err != nil {
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
	pdf, pdfErr := readResumeUpload(r)
	if exp.Company == "" || exp.Role == "" || pdfErr != "" {
		msg := pdfErr
		if exp.Company == "" || exp.Role == "" {
			msg = "Company and role are required."
		}
		a.renderExperienceForm(w, r, exp, id == 0, msg)
		return
	}
	newKey := experienceRoleKey(exp.Company, exp.Role)
	if len(pdf) > 0 && newKey == "" {
		a.renderExperienceForm(w, r, exp, id == 0, "Company and role need letters or numbers so a resume can be keyed to them.")
		return
	}
	if len(pdf) > 0 && a.CourseStorage == nil {
		a.renderExperienceForm(w, r, exp, id == 0, "Storage is not configured, so the resume cannot be saved.")
		return
	}

	ctx := r.Context()
	var oldKey string
	if id != 0 {
		old, err := a.Experience.GetByID(ctx, id)
		if err != nil {
			if err == repo.ErrNotFound {
				a.NotFound(w, r)
				return
			}
			a.serverError(w, "load experience", err)
			return
		}
		oldKey = experienceRoleKey(old.Company, old.Role)
	}
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
	if err := a.syncRoleResume(ctx, oldKey, newKey, resumeLabel(exp.Role, exp.Company), exp.SortOrder, pdf); err != nil {
		a.serverError(w, "save resume", err)
		return
	}
	http.Redirect(w, r, "/admin/experience", http.StatusSeeOther)
}

func (a *App) renderExperienceForm(w http.ResponseWriter, r *http.Request, exp models.Experience, isNew bool, msg string) {
	resume, _ := a.resumeForExperience(r.Context(), exp)
	a.View.Render(w, http.StatusUnprocessableEntity, "admin_experience_form", AdminExperienceFormData{
		BaseData:   a.base(r, "Experience — Admin", "", "admin"),
		Experience: exp,
		Resume:     resume,
		IsNew:      isNew,
		Error:      msg,
	})
}

func (a *App) AdminExperienceDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	ctx := r.Context()
	exp, err := a.Experience.GetByID(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load experience", err)
		return
	}
	key := experienceRoleKey(exp.Company, exp.Role)
	if key != "" {
		used, err := a.experienceUsesRoleKey(ctx, key, id)
		if err != nil {
			a.serverError(w, "load experience", err)
			return
		}
		if !used {
			if err := a.removeResume(ctx, key); err != nil {
				a.serverError(w, "delete resume", err)
				return
			}
		}
	}
	if err := a.Experience.Delete(ctx, id); err != nil {
		a.serverError(w, "delete experience", err)
		return
	}
	http.Redirect(w, r, "/admin/experience", http.StatusSeeOther)
}

func experienceRoleKey(company, role string) string {
	return slugify(company + " " + role)
}

func resumeLabel(role, company string) string {
	return role + " · " + company
}

func readResumeUpload(r *http.Request) ([]byte, string) {
	file, _, err := r.FormFile("resume_file")
	if err == http.ErrMissingFile {
		return nil, ""
	}
	if err != nil {
		return nil, "Could not read the resume file."
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, maxCourseUploadBytes+1))
	if err != nil {
		return nil, "Could not read the resume file."
	}
	if len(b) == 0 {
		return nil, ""
	}
	if len(b) > maxCourseUploadBytes {
		return nil, "Resume PDF must be 8MB or smaller."
	}
	if len(b) < 5 || string(b[:5]) != "%PDF-" {
		return nil, "Resume must be a PDF."
	}
	return b, ""
}

func (a *App) resumeForExperience(ctx context.Context, exp models.Experience) (models.RoleResume, error) {
	key := experienceRoleKey(exp.Company, exp.Role)
	if key == "" {
		return models.RoleResume{}, nil
	}
	row, err := a.Resumes.Get(ctx, key)
	if err == repo.ErrNotFound {
		return models.RoleResume{}, nil
	}
	return row, err
}

func (a *App) syncRoleResume(ctx context.Context, oldKey, newKey, label string, sort int, pdf []byte) error {
	if newKey == "" {
		return nil
	}
	if len(pdf) > 0 {
		if a.CourseStorage == nil {
			return fmt.Errorf("storage is not configured")
		}
		path, err := storage.ResumeObject(newKey)
		if err != nil {
			return err
		}
		if err := a.CourseStorage.Put(ctx, path, "application/pdf", pdf); err != nil {
			return err
		}
		if err := a.Resumes.Upsert(ctx, models.RoleResume{
			RoleKey: newKey, Label: label, ObjectPath: path, SortOrder: sort,
		}); err != nil {
			return err
		}
		if oldKey != "" && oldKey != newKey {
			return a.dropResumeIfUnused(ctx, oldKey)
		}
		return nil
	}
	if oldKey == "" || oldKey == newKey {
		if oldKey != newKey {
			return nil
		}
		row, err := a.Resumes.Get(ctx, newKey)
		if err == repo.ErrNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		row.Label = label
		row.SortOrder = sort
		return a.Resumes.Upsert(ctx, row)
	}
	row, err := a.Resumes.Get(ctx, oldKey)
	if err == repo.ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if a.CourseStorage == nil {
		return fmt.Errorf("storage is not configured")
	}
	rc, err := a.CourseStorage.Open(ctx, row.ObjectPath)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(rc, maxCourseUploadBytes+1))
	rc.Close()
	if err != nil {
		return err
	}
	if len(raw) > maxCourseUploadBytes {
		return fmt.Errorf("stored resume exceeds %d bytes", maxCourseUploadBytes)
	}
	path, err := storage.ResumeObject(newKey)
	if err != nil {
		return err
	}
	if err := a.CourseStorage.Put(ctx, path, "application/pdf", raw); err != nil {
		return err
	}
	if err := a.Resumes.Upsert(ctx, models.RoleResume{
		RoleKey: newKey, Label: label, ObjectPath: path, SortOrder: sort,
	}); err != nil {
		return err
	}
	return a.dropResumeIfUnused(ctx, oldKey)
}

func (a *App) dropResumeIfUnused(ctx context.Context, key string) error {
	used, err := a.experienceUsesRoleKey(ctx, key, 0)
	if err != nil || used {
		return err
	}
	return a.removeResume(ctx, key)
}

func (a *App) removeResume(ctx context.Context, key string) error {
	row, err := a.Resumes.Get(ctx, key)
	if err == repo.ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if a.CourseStorage != nil && row.ObjectPath != "" {
		if err := a.CourseStorage.Delete(ctx, row.ObjectPath); err != nil {
			return err
		}
	}
	return a.Resumes.Delete(ctx, key)
}

func (a *App) experienceUsesRoleKey(ctx context.Context, key string, exceptID int64) (bool, error) {
	list, err := a.Experience.List(ctx)
	if err != nil {
		return false, err
	}
	for _, e := range list {
		if e.ID != exceptID && experienceRoleKey(e.Company, e.Role) == key {
			return true, nil
		}
	}
	return false, nil
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
