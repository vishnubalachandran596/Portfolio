package repo

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/db"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

// quizTestDB opens DATABASE_URL (skipping when unset) and creates a throw-away
// course with a 100-question bank and a user; everything is deleted at cleanup.
func quizTestDB(t *testing.T) (conn *sql.DB, courseID, userID int64) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	conn, err := db.Open(url)
	if err != nil {
		t.Skipf("db unavailable: %v", err)
	}
	ctx := context.Background()
	tag := fmt.Sprintf("zzw2b%d", time.Now().UnixNano())
	if err := conn.QueryRowContext(ctx, `INSERT INTO courses (slug, title) VALUES ($1,$1) RETURNING id`, tag).Scan(&courseID); err != nil {
		t.Skipf("cannot create course (migrations not applied?): %v", err)
	}
	if err := conn.QueryRowContext(ctx, `INSERT INTO users (email, password_hash, name) VALUES ($1,'x',$1) RETURNING id`, tag+"@test.invalid").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Exec(`DELETE FROM courses WHERE id = $1`, courseID)
		conn.Exec(`DELETE FROM users WHERE id = $1`, userID)
		conn.Close()
	})
	for i := 1; i <= 100; i++ {
		if _, err := conn.ExecContext(ctx, `INSERT INTO quiz_questions (course_id, module, question, option_a, option_b, option_c, option_d, correct_option, sort_order)
			VALUES ($1,$2,$3,'a','b','c','d','B',$4)`, courseID, fmt.Sprintf("M%d", i%4), fmt.Sprintf("q%d", i), i); err != nil {
			t.Fatal(err)
		}
	}
	return conn, courseID, userID
}

func pickerFor(bank []models.QuizQuestion) func(int) (int, []int64) {
	return func(n int) (int, []int64) {
		set, qs := models.QuizSetFor(bank, n)
		ids := make([]int64, len(qs))
		for i, q := range qs {
			ids[i] = q.ID
		}
		return set, ids
	}
}

func TestQuizSetsAssignedAndAttemptCap(t *testing.T) {
	conn, courseID, userID := quizTestDB(t)
	ctx := context.Background()
	qr, ar, gr := NewQuizQuestionRepo(conn), NewQuizAttemptRepo(conn), NewQuizGrantRepo(conn)

	if err := qr.AssignSets(ctx, courseID); err != nil {
		t.Fatal(err)
	}
	bank, _ := qr.ListByCourse(ctx, courseID)
	counts := map[int]int{}
	for _, q := range bank {
		counts[q.SetNo]++
	}
	for s := 1; s <= 5; s++ {
		if counts[s] != 20 {
			t.Fatalf("set %d has %d questions: %v", s, counts[s], counts)
		}
	}
	// Re-running the assignment is stable.
	if err := qr.AssignSets(ctx, courseID); err != nil {
		t.Fatal(err)
	}
	again, _ := qr.ListByCourse(ctx, courseID)
	for i := range bank {
		if bank[i].SetNo != again[i].SetNo {
			t.Fatal("AssignSets is not stable")
		}
	}

	who := QuizLearner{UserID: &userID, Name: "T", Email: "t@test.invalid"}
	pick := pickerFor(bank)
	used := map[int64]bool{}
	for n := 1; n <= 5; n++ {
		a, resumed, err := ar.Start(ctx, courseID, who, pick)
		if err != nil || resumed || a.AttemptNo != n || a.SetNo != n || len(a.QuestionIDs) != 20 {
			t.Fatalf("start %d: %+v resumed=%v err=%v", n, a, resumed, err)
		}
		for _, id := range a.QuestionIDs {
			if used[id] {
				t.Fatalf("question %d served twice across attempts", id)
			}
			used[id] = true
		}
		// A second start while open resumes, and does not consume an attempt.
		if r2, resumed, err := ar.Start(ctx, courseID, who, pick); err != nil || !resumed || r2.ID != a.ID {
			t.Fatalf("expected resume, got %+v %v %v", r2, resumed, err)
		}
		answers := map[int64]string{}
		for _, id := range a.QuestionIDs {
			answers[id] = "B"
		}
		done, err := ar.Submit(ctx, courseID, who, answers, "CERT"+fmt.Sprint(courseID, n))
		if err != nil || !done.Passed || done.ScorePercent != 100 || done.CertificateCode == "" {
			t.Fatalf("submit %d: %+v %v", n, done, err)
		}
		// Cannot resubmit.
		if _, err := ar.Submit(ctx, courseID, who, answers, "X"); err != ErrAlreadySubmitted {
			t.Fatalf("resubmit: %v", err)
		}
	}
	if _, _, err := ar.Start(ctx, courseID, who, pick); err != ErrNoAttemptsLeft {
		t.Fatalf("6th start: %v", err)
	}
	// Direct SQL cannot bypass the cap either (trigger + unique).
	ins := `INSERT INTO quiz_attempts (course_id, user_id, name, email, total_questions, correct_answers, score_percent, passed, attempt_no)
	        VALUES ($1,$2,'n','e',1,0,0,false,$3)`
	if _, err := conn.ExecContext(ctx, ins, courseID, userID, 6); err == nil {
		t.Fatal("direct insert of attempt 6 should fail")
	}
	// An admin grant raises the cap by exactly the grant.
	if err := gr.Set(ctx, userID, courseID, 1); err != nil {
		t.Fatal(err)
	}
	a6, _, err := ar.Start(ctx, courseID, who, pick)
	if err != nil || a6.AttemptNo != 6 || a6.SetNo != 1 {
		t.Fatalf("granted 6th attempt: %+v %v", a6, err)
	}
	// Abandoned: never submitted, yet it counts.
	if u, l, _ := ar.Usage(ctx, courseID, who); u != 6 || l != 6 {
		t.Fatalf("usage %d/%d", u, l)
	}
	if _, err := conn.ExecContext(ctx, ins, courseID, userID, 7); err == nil {
		t.Fatal("direct insert of attempt 7 should fail with grant of 1")
	}
}

func TestQuizConcurrentStartsCannotExceedCap(t *testing.T) {
	conn, courseID, userID := quizTestDB(t)
	ctx := context.Background()
	qr, ar := NewQuizQuestionRepo(conn), NewQuizAttemptRepo(conn)
	if err := qr.AssignSets(ctx, courseID); err != nil {
		t.Fatal(err)
	}
	bank, _ := qr.ListByCourse(ctx, courseID)
	pick := pickerFor(bank)
	who := QuizLearner{UserID: &userID, Name: "T", Email: "c@test.invalid"}

	// Start fresh attempts concurrently, abandoning each (the open attempt
	// is abandoned by marking it submitted straight away, as a fast learner).
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, denied := 0, 0
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, resumed, err := ar.Start(ctx, courseID, who, pick)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == ErrNoAttemptsLeft:
				denied++
			case err != nil:
				t.Errorf("start: %v", err)
			case !resumed:
				ok++
				_ = a
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("30 concurrent starts on an empty history created %d attempts, want 1 (rest resume the open one)", ok)
	}

	// Now burn through to the cap with concurrent submit+start cycles.
	for round := 0; round < 12; round++ {
		var wg2 sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg2.Add(1)
			go func() {
				defer wg2.Done()
				ar.Start(ctx, courseID, who, pick)
			}()
		}
		wg2.Wait()
		ar.Submit(ctx, courseID, who, map[int64]string{}, "")
	}
	var n int
	conn.QueryRowContext(ctx, `SELECT count(*) FROM quiz_attempts WHERE user_id=$1 AND course_id=$2`, userID, courseID).Scan(&n)
	if n != models.QuizMaxAttempts {
		t.Fatalf("rows = %d, want exactly %d", n, models.QuizMaxAttempts)
	}
}

func TestQuizSubmitScoresServerSide(t *testing.T) {
	conn, courseID, userID := quizTestDB(t)
	ctx := context.Background()
	qr, ar := NewQuizQuestionRepo(conn), NewQuizAttemptRepo(conn)
	qr.AssignSets(ctx, courseID)
	bank, _ := qr.ListByCourse(ctx, courseID)
	who := QuizLearner{UserID: &userID, Name: "T", Email: "s@test.invalid"}
	a, _, err := ar.Start(ctx, courseID, who, pickerFor(bank))
	if err != nil {
		t.Fatal(err)
	}
	// Answers for questions outside the stored set are ignored; 13/20 right = 65% fails.
	answers := map[int64]string{}
	for i, id := range a.QuestionIDs {
		if i < 13 {
			answers[id] = "B"
		} else {
			answers[id] = "A"
		}
	}
	for _, q := range bank {
		if q.SetNo != a.SetNo {
			answers[q.ID] = "B"
		}
	}
	done, err := ar.Submit(ctx, courseID, who, answers, "CODE")
	if err != nil || done.Passed || done.ScorePercent != 65 || done.CorrectAnswers != 13 || done.TotalQuestions != 20 || done.CertificateCode != "" {
		t.Fatalf("%+v %v", done, err)
	}
}
