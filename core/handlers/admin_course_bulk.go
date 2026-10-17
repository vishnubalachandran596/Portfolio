package handlers

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/storage"
)

// Bulk lesson upload: one .zip of courses/{slug}.html files instead of one
// upload per course on each edit form. Each entry is matched to a course by
// its file name (java-fundamentals.html → slug java-fundamentals), stored at
// the same private Storage path AdminCourseSave uses, and its outline is
// re-derived from the bytes just stored. Unknown slugs are reported, never
// created — a course row is still made through the normal form.

const maxBulkUploadBytes = 8 << 20

type BulkUploadResult struct {
	File   string
	Slug   string
	Status string // "updated", "skipped"
	Note   string
}

type AdminCourseBulkData struct {
	BaseData
	Results []BulkUploadResult
	Error   string
}

func (a *App) AdminCourseBulkPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_course_bulk", AdminCourseBulkData{
		BaseData: a.base(r, "Bulk upload lessons — Admin", "", "admin"),
	})
}

func (a *App) AdminCourseBulkUpload(w http.ResponseWriter, r *http.Request) {
	render := func(status int, d AdminCourseBulkData) {
		d.BaseData = a.base(r, "Bulk upload lessons — Admin", "", "admin")
		a.View.Render(w, status, "admin_course_bulk", d)
	}
	if a.CourseStorage == nil {
		render(http.StatusServiceUnavailable, AdminCourseBulkData{Error: "Course storage isn't configured on this deployment."})
		return
	}
	if err := r.ParseMultipartForm(maxBulkUploadBytes); err != nil {
		render(http.StatusBadRequest, AdminCourseBulkData{Error: "Couldn't read the upload — the zip may be larger than 8 MB."})
		return
	}
	file, _, err := r.FormFile("archive")
	if err != nil {
		render(http.StatusBadRequest, AdminCourseBulkData{Error: "Choose a .zip file of lesson HTML files."})
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBulkUploadBytes+1))
	if err != nil || len(raw) > maxBulkUploadBytes {
		render(http.StatusBadRequest, AdminCourseBulkData{Error: "The zip is larger than 8 MB."})
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		render(http.StatusBadRequest, AdminCourseBulkData{Error: "That file isn't a valid .zip archive."})
		return
	}

	ctx := r.Context()
	var results []BulkUploadResult
	for _, f := range zr.File {
		name := path.Base(f.Name)
		if f.FileInfo().IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(f.Name, "__MACOSX") {
			continue
		}
		res := BulkUploadResult{File: f.Name}
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".html") && !strings.HasSuffix(lower, ".htm") {
			res.Status, res.Note = "skipped", "not an .html file"
			results = append(results, res)
			continue
		}
		res.Slug = strings.TrimSuffix(strings.TrimSuffix(name, path.Ext(name)), ".")
		course, err := a.Courses.GetBySlug(ctx, res.Slug)
		if err != nil {
			res.Status, res.Note = "skipped", "no course with this slug"
			results = append(results, res)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			res.Status, res.Note = "skipped", "couldn't read this entry"
			results = append(results, res)
			continue
		}
		body, err := io.ReadAll(io.LimitReader(rc, maxCourseUploadBytes+1))
		rc.Close()
		if err != nil || len(body) == 0 || len(body) > maxCourseUploadBytes {
			res.Status, res.Note = "skipped", "empty or larger than 8 MB"
			results = append(results, res)
			continue
		}
		body = storage.NormalizeLesson(body) // docs/DESIGN.md "Content updates"
		objectPath, err := storage.CourseObject(course.Slug)
		if err != nil {
			res.Status, res.Note = "skipped", "invalid slug"
			results = append(results, res)
			continue
		}
		if err := a.CourseStorage.Put(ctx, objectPath, "text/html", body); err != nil {
			res.Status, res.Note = "skipped", "storage upload failed: "+err.Error()
			results = append(results, res)
			continue
		}
		if err := a.Courses.UpdateContentObject(ctx, course.ID, objectPath); err != nil {
			res.Status, res.Note = "skipped", "stored, but saving the path failed"
			results = append(results, res)
			continue
		}
		outline := storage.LessonOutline(body)
		_ = a.Courses.UpdateOutline(ctx, course.ID, strings.Join(outline, "\n"))
		res.Status = "updated"
		res.Note = course.Title + " · " + strconv.Itoa(len(outline)) + " sections"
		results = append(results, res)
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Status > results[j].Status })
	if len(results) == 0 {
		render(http.StatusBadRequest, AdminCourseBulkData{Error: "The zip had no files in it."})
		return
	}
	a.markChatbotReindexPending(ctx)
	render(http.StatusOK, AdminCourseBulkData{Results: results})
}
