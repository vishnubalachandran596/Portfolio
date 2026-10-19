package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestParseQuizImport(t *testing.T) {
	good := `[{"module":"M1","question":"What is 2+2?","a":"3","b":"4","c":"5","d":"6","correct":"b"},
	          {"module":"M1","question":"Capital of France?","a":"Rome","b":"Paris","c":"Oslo","d":"Bern","correct":"B"}]`
	qs, errs := parseQuizImport([]byte(good), map[string]bool{}, 7)
	if len(errs) != 0 || len(qs) != 2 || qs[0].CorrectOption != "B" || qs[0].CourseID != 7 {
		t.Fatalf("good import: %v %+v", errs, qs)
	}

	bad := map[string]string{
		"non-ascii":    `[{"module":"","question":"It’s","a":"1","b":"2","c":"3","d":"4","correct":"A"}]`,
		"bad correct":  `[{"module":"","question":"Q","a":"1","b":"2","c":"3","d":"4","correct":"E"}]`,
		"missing opt":  `[{"module":"","question":"Q","a":"1","b":"2","c":"3","d":"","correct":"A"}]`,
		"same options": `[{"module":"","question":"Q","a":"1","b":"1","c":"3","d":"4","correct":"A"}]`,
		"dup in file":  `[{"question":"Q","a":"1","b":"2","c":"3","d":"4","correct":"A"},{"question":"q ","a":"1","b":"2","c":"3","d":"5","correct":"A"}]`,
		"unknown key":  `[{"question":"Q","a":"1","b":"2","c":"3","d":"4","correct":"A","x":1}]`,
		"not array":    `{"question":"Q"}`,
	}
	for name, body := range bad {
		if qs, errs := parseQuizImport([]byte(body), map[string]bool{}, 1); len(errs) == 0 || qs != nil {
			t.Errorf("%s: expected rejection, got %+v", name, qs)
		}
	}
	// Duplicate against the stored bank.
	one := `[{"question":"What is 2+2?","a":"3","b":"4","c":"5","d":"6","correct":"A"}]`
	if _, errs := parseQuizImport([]byte(one), map[string]bool{"what is 2+2?": true}, 1); len(errs) == 0 {
		t.Error("duplicate of stored question should be rejected")
	}
}

// The quiz-taking page must never carry the answer key. The view type has no
// correct-answer field; this renders it with a distinctive answer and checks
// the HTML.
func TestQuizPageDoesNotLeakAnswers(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	data := QuizPageData{
		BaseData: BaseData{SiteName: "S", Nav: NavLinks{Home: "/", Courses: "/courses"}},
		Course:   models.Course{Slug: "go", Title: "Go"},
		State:    "taking", AttemptNo: 1, Limit: 5, Used: 1, Left: 4, PassPct: 70,
		Questions: []quizQuestionView{{ID: 41, Question: "Pick one", A: "alpha", B: "bravo", C: "charlie", D: "delta"}},
	}
	rec := httptest.NewRecorder()
	r.Render(rec, 200, "quiz", data)
	body := rec.Body.String()
	for _, want := range []string{"alpha", "bravo", "name=\"q41\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing %q", want)
		}
	}
	for _, leak := range []string{"correct", "Correct", "checked", "answer-key", "data-answer"} {
		if strings.Contains(body, leak) {
			t.Errorf("rendered quiz page contains %q", leak)
		}
	}
	// Result page shows attempts left.
	rec = httptest.NewRecorder()
	r.Render(rec, 200, "quiz_result", QuizResultData{
		BaseData: data.BaseData, Course: data.Course, PassPct: 70, Used: 2, Left: 3,
		Attempt: models.QuizAttempt{TotalQuestions: 20, CorrectAnswers: 10, ScorePercent: 50},
	})
	if out := rec.Body.String(); !strings.Contains(out, "Attempts left") || !strings.Contains(out, "50%") {
		t.Errorf("result page missing score/attempts left")
	}
}
