package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

const maxQuizImportBytes = 2 << 20

// quizImportItem is one entry of the bulk-import JSON array.
type quizImportItem struct {
	Module   string `json:"module"`
	Question string `json:"question"`
	A        string `json:"a"`
	B        string `json:"b"`
	C        string `json:"c"`
	D        string `json:"d"`
	Correct  string `json:"correct"`
}

// isPlainASCII reports whether s holds only printable ASCII plus newline
// and tab: typographic quotes, dashes and other non-ASCII are rejected so
// the bank stays uniform.
func isPlainASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' || c == '\t' || (c >= 0x20 && c < 0x7f) {
			continue
		}
		return false
	}
	return true
}

// parseQuizImport validates a bulk-import payload against the existing
// bank. It returns the questions to insert, or a list of human-readable
// errors (capped) - nothing is inserted unless the whole file is valid.
func parseQuizImport(data []byte, existing map[string]bool, courseID int64) (qs []models.QuizQuestion, errs []string) {
	var items []quizImportItem
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&items); err != nil {
		return nil, []string{"Not a valid JSON array of question objects: " + err.Error()}
	}
	if len(items) == 0 {
		return nil, []string{"The file contains no questions."}
	}
	if len(existing)+len(items) > models.QuizBankSize {
		return nil, []string{fmt.Sprintf("The bank holds at most %d questions per course (%d already stored, %d in the file).",
			models.QuizBankSize, len(existing), len(items))}
	}
	const maxErrs = 20
	addErr := func(i int, msg string) {
		if len(errs) < maxErrs {
			errs = append(errs, fmt.Sprintf("Item %d: %s", i+1, msg))
		}
	}
	seen := map[string]bool{}
	for i, it := range items {
		it.Module = strings.TrimSpace(it.Module)
		it.Question = strings.TrimSpace(it.Question)
		it.A, it.B, it.C, it.D = strings.TrimSpace(it.A), strings.TrimSpace(it.B), strings.TrimSpace(it.C), strings.TrimSpace(it.D)
		correct := strings.ToUpper(strings.TrimSpace(it.Correct))
		if it.Question == "" || it.A == "" || it.B == "" || it.C == "" || it.D == "" {
			addErr(i, "question and all four options a-d are required.")
			continue
		}
		if correct != "A" && correct != "B" && correct != "C" && correct != "D" {
			addErr(i, "correct must be A, B, C or D.")
			continue
		}
		if !isPlainASCII(it.Module + it.Question + it.A + it.B + it.C + it.D) {
			addErr(i, "contains non-ASCII characters (use plain quotes and hyphens).")
			continue
		}
		if it.A == it.B || it.A == it.C || it.A == it.D || it.B == it.C || it.B == it.D || it.C == it.D {
			addErr(i, "two options are identical.")
			continue
		}
		key := strings.ToLower(it.Question)
		if existing[key] {
			addErr(i, "question already exists in this course's bank.")
			continue
		}
		if seen[key] {
			addErr(i, "duplicate of an earlier question in the file.")
			continue
		}
		seen[key] = true
		qs = append(qs, models.QuizQuestion{
			CourseID: courseID, Module: it.Module, Question: it.Question,
			OptionA: it.A, OptionB: it.B, OptionC: it.C, OptionD: it.D,
			CorrectOption: correct, SortOrder: i + 1,
		})
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return qs, nil
}

func (a *App) quizAdminCourse(w http.ResponseWriter, r *http.Request) (models.Course, bool) {
	courseID, err := strconv.ParseInt(r.PathValue("courseID"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return models.Course{}, false
	}
	course, err := a.Courses.GetByID(r.Context(), courseID)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
		} else {
			a.serverError(w, "load course", err)
		}
		return models.Course{}, false
	}
	return course, true
}

// AdminQuizAssignSets (re)deals the bank into the five sets.
func (a *App) AdminQuizAssignSets(w http.ResponseWriter, r *http.Request) {
	course, ok := a.quizAdminCourse(w, r)
	if !ok {
		return
	}
	if err := a.QuizQuestions.AssignSets(r.Context(), course.ID); err != nil {
		a.serverError(w, "assign quiz sets", err)
		return
	}
	http.Redirect(w, r, "/admin/courses/"+strconv.FormatInt(course.ID, 10)+"/quiz?msg=sets", http.StatusSeeOther)
}

// AdminQuizImport bulk-imports questions from an uploaded JSON array.
func (a *App) AdminQuizImport(w http.ResponseWriter, r *http.Request) {
	course, ok := a.quizAdminCourse(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxQuizImportBytes+1024)
	if err := r.ParseMultipartForm(maxQuizImportBytes); err != nil {
		a.renderQuizList(w, r, course, http.StatusBadRequest, []string{"Upload a JSON file (max 2 MB)."}, "")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		a.renderQuizList(w, r, course, http.StatusBadRequest, []string{"Choose a JSON file to upload."}, "")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxQuizImportBytes+1))
	if err != nil || len(data) > maxQuizImportBytes {
		a.renderQuizList(w, r, course, http.StatusBadRequest, []string{"The file is too large (max 2 MB)."}, "")
		return
	}
	existing, err := a.QuizQuestions.ExistingQuestions(r.Context(), course.ID)
	if err != nil {
		a.serverError(w, "load existing quiz questions", err)
		return
	}
	qs, errs := parseQuizImport(data, existing, course.ID)
	if len(errs) > 0 {
		a.renderQuizList(w, r, course, http.StatusUnprocessableEntity, errs, "")
		return
	}
	n, err := a.QuizQuestions.ImportBulk(r.Context(), course.ID, qs)
	if err != nil {
		a.serverError(w, "import quiz questions", err)
		return
	}
	http.Redirect(w, r, "/admin/courses/"+strconv.FormatInt(course.ID, 10)+"/quiz?msg=imported&n="+strconv.Itoa(n), http.StatusSeeOther)
}

// AdminUserQuizGrant sets the extra quiz attempts for one learner and course.
func (a *App) AdminUserQuizGrant(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if _, err := a.Users.GetByID(r.Context(), userID); err != nil {
		a.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	courseID, err := strconv.ParseInt(r.FormValue("course_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid course", http.StatusBadRequest)
		return
	}
	if _, err := a.Courses.GetByID(r.Context(), courseID); err != nil {
		http.Error(w, "invalid course", http.StatusBadRequest)
		return
	}
	extra, err := strconv.Atoi(strings.TrimSpace(r.FormValue("extra")))
	if err != nil || extra < 0 || extra > 50 {
		http.Error(w, "extra must be a number from 0 to 50", http.StatusBadRequest)
		return
	}
	if err := a.QuizGrants.Set(r.Context(), userID, courseID, extra); err != nil {
		a.serverError(w, "set quiz grant", err)
		return
	}
	http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(userID, 10)+"#quiz-attempts", http.StatusSeeOther)
}
