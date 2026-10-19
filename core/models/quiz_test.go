package models

import "testing"

func bank(n int, assign bool) []QuizQuestion {
	var qs []QuizQuestion
	for i := 0; i < n; i++ {
		q := QuizQuestion{ID: int64(i + 1), CorrectOption: "B"}
		if assign {
			q.SetNo = i%QuizSets + 1
		}
		qs = append(qs, q)
	}
	return qs
}

func TestQuizSetForNonOverlapping(t *testing.T) {
	for _, assign := range []bool{true, false} {
		b := bank(100, assign)
		seen := map[int64]int{}
		for attempt := 1; attempt <= 5; attempt++ {
			set, qs := QuizSetFor(b, attempt)
			if set != attempt {
				t.Fatalf("attempt %d served set %d", attempt, set)
			}
			if len(qs) != QuizSetSize {
				t.Fatalf("assign=%v attempt %d: %d questions, want %d", assign, attempt, len(qs), QuizSetSize)
			}
			for _, q := range qs {
				seen[q.ID]++
			}
		}
		if len(seen) != 100 {
			t.Fatalf("assign=%v: sets cover %d questions, want 100 (overlap or gap)", assign, len(seen))
		}
		for id, n := range seen {
			if n != 1 {
				t.Fatalf("question %d appears in %d sets", id, n)
			}
		}
	}
}

func TestQuizSetForDegradesAndWraps(t *testing.T) {
	if _, qs := QuizSetFor(bank(3, false), 2); len(qs) != 3 {
		t.Fatalf("tiny bank should serve everything, got %d", len(qs))
	}
	if _, qs := QuizSetFor(bank(37, false), 1); len(qs) == 0 || len(qs) > 8 {
		t.Fatalf("37-question bank set 1 has %d questions", len(qs))
	}
	// A granted 6th attempt repeats set 1.
	if set, _ := QuizSetFor(bank(100, true), 6); set != 1 {
		t.Fatalf("attempt 6 served set %d", set)
	}
	// Assigned sets with an empty target set fall back to dealing.
	b := bank(50, true)
	for i := range b {
		if b[i].SetNo == 3 {
			b[i].SetNo = 1
		}
	}
	if _, qs := QuizSetFor(b, 3); len(qs) == 0 {
		t.Fatal("empty assigned set should fall back")
	}
}

func TestScoreQuiz(t *testing.T) {
	qs := bank(10, true)
	answers := map[int64]string{}
	for i := 1; i <= 7; i++ {
		answers[int64(i)] = "b"
	}
	c, total, pct, passed := ScoreQuiz(qs, answers)
	if c != 7 || total != 10 || pct != 70 || !passed {
		t.Fatalf("got %d/%d %d%% passed=%v", c, total, pct, passed)
	}
	answers[7] = "A"
	if _, _, pct, passed = ScoreQuiz(qs, answers); pct != 60 || passed {
		t.Fatalf("60%% should fail, got %d passed=%v", pct, passed)
	}
	if _, _, _, passed = ScoreQuiz(nil, nil); passed {
		t.Fatal("empty set must not pass")
	}
}
