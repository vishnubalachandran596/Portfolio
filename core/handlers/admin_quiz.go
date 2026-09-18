package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

type AdminQuizListData struct {
	BaseData
	Course    models.Course
	Questions []models.QuizQuestion
}

func (a *App) AdminQuizList(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.ParseInt(r.PathValue("courseID"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	course, err := a.Courses.GetByID(r.Context(), courseID)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load course", err)
		return
	}
	questions, err := a.QuizQuestions.ListByCourse(r.Context(), courseID)
	if err != nil {
		a.serverError(w, "load quiz questions", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_quiz_list", AdminQuizListData{
		BaseData:  a.base(r, "Quiz — "+course.Title+" — Admin", "", "admin"),
		Course:    course,
		Questions: questions,
	})
}

type AdminQuizFormData struct {
	BaseData
	Course   models.Course
	Question models.QuizQuestion
	IsNew    bool
	Error    string
}

func (a *App) AdminQuizNewPage(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.ParseInt(r.PathValue("courseID"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	course, err := a.Courses.GetByID(r.Context(), courseID)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load course", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_quiz_form", AdminQuizFormData{
		BaseData: a.base(r, "New Quiz Question — Admin", "", "admin"),
		Course:   course,
		Question: models.QuizQuestion{CourseID: courseID, CorrectOption: "A"},
		IsNew:    true,
	})
}

func (a *App) AdminQuizEditPage(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.ParseInt(r.PathValue("courseID"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	course, err := a.Courses.GetByID(r.Context(), courseID)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load course", err)
		return
	}
	question, err := a.QuizQuestions.GetByID(r.Context(), id)
	if err != nil || question.CourseID != courseID {
		if err == repo.ErrNotFound || question.CourseID != courseID {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load quiz question", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_quiz_form", AdminQuizFormData{
		BaseData: a.base(r, "Edit Quiz Question — Admin", "", "admin"),
		Course:   course,
		Question: question,
	})
}

func (a *App) AdminQuizSave(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.ParseInt(r.PathValue("courseID"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	course, err := a.Courses.GetByID(r.Context(), courseID)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load course", err)
		return
	}
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
	q := models.QuizQuestion{
		ID:            id,
		CourseID:      courseID,
		Question:      strings.TrimSpace(r.FormValue("question")),
		OptionA:       strings.TrimSpace(r.FormValue("option_a")),
		OptionB:       strings.TrimSpace(r.FormValue("option_b")),
		OptionC:       strings.TrimSpace(r.FormValue("option_c")),
		OptionD:       strings.TrimSpace(r.FormValue("option_d")),
		CorrectOption: strings.ToUpper(strings.TrimSpace(r.FormValue("correct_option"))),
		SortOrder:     sortOrder,
	}

	formErr := ""
	switch {
	case q.Question == "":
		formErr = "A question is required."
	case q.OptionA == "" || q.OptionB == "" || q.OptionC == "" || q.OptionD == "":
		formErr = "All four options are required."
	case q.CorrectOption != "A" && q.CorrectOption != "B" && q.CorrectOption != "C" && q.CorrectOption != "D":
		formErr = "Pick which option is correct."
	}
	if formErr != "" {
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_quiz_form", AdminQuizFormData{
			BaseData: a.base(r, "Quiz Question — Admin", "", "admin"),
			Course:   course,
			Question: q,
			IsNew:    id == 0,
			Error:    formErr,
		})
		return
	}

	ctx := r.Context()
	if id == 0 {
		_, err = a.QuizQuestions.Create(ctx, q)
	} else {
		err = a.QuizQuestions.Update(ctx, q)
	}
	if err != nil {
		a.serverError(w, "save quiz question", err)
		return
	}
	http.Redirect(w, r, "/admin/courses/"+strconv.FormatInt(courseID, 10)+"/quiz", http.StatusSeeOther)
}

func (a *App) AdminQuizDelete(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.ParseInt(r.PathValue("courseID"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.QuizQuestions.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete quiz question", err)
		return
	}
	http.Redirect(w, r, "/admin/courses/"+strconv.FormatInt(courseID, 10)+"/quiz", http.StatusSeeOther)
}
