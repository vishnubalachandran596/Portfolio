package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

// ExistingQuestions returns the lower-cased trimmed question texts already in
// a course's bank, for duplicate detection on bulk import.
func (r *QuizQuestionRepo) ExistingQuestions(ctx context.Context, courseID int64) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT question FROM quiz_questions WHERE course_id = $1`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			return nil, err
		}
		out[strings.ToLower(strings.TrimSpace(q))] = true
	}
	return out, rows.Err()
}

// ImportBulk inserts the questions in one transaction (all or nothing) and
// returns how many rows were actually added. The (course_id, question) unique
// constraint is the last line of defence against duplicates.
func (r *QuizQuestionRepo) ImportBulk(ctx context.Context, courseID int64, qs []models.QuizQuestion) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, q := range qs {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO quiz_questions (course_id, module, question, option_a, option_b, option_c, option_d, correct_option, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (course_id, question) DO NOTHING`,
			courseID, q.Module, q.Question, q.OptionA, q.OptionB, q.OptionC, q.OptionD, q.CorrectOption, q.SortOrder)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	return added, tx.Commit()
}

// AssignSets splits the course bank into QuizSets non-overlapping sets: the
// first QuizBankSize questions in a stable (hash of id) order are kept, then
// dealt round-robin in module order so every set spans the modules evenly.
// Questions beyond QuizBankSize get no set. Attempts already started keep
// their stored question ids, so this only affects future attempts.
func (r *QuizQuestionRepo) AssignSets(ctx context.Context, courseID int64) error {
	_, err := r.db.ExecContext(ctx, `
		WITH ranked AS (
			SELECT id, module, row_number() OVER (ORDER BY md5(id::text || ':' || course_id::text), id) AS g
			  FROM quiz_questions WHERE course_id = $1),
		dealt AS (
			SELECT id, row_number() OVER (ORDER BY module, md5(id::text), id) AS d
			  FROM ranked WHERE g <= $2)
		UPDATE quiz_questions q
		   SET set_no = CASE WHEN d.d IS NULL THEN NULL ELSE ((d.d - 1) % $3 + 1)::smallint END
		  FROM ranked rk LEFT JOIN dealt d ON d.id = rk.id
		 WHERE q.id = rk.id`, courseID, models.QuizBankSize, models.QuizSets)
	return err
}

// ListByIDs loads the given questions of a course, for showing an attempt's
// stored set.
func (r *QuizQuestionRepo) ListByIDs(ctx context.Context, courseID int64, ids []int64) ([]models.QuizQuestion, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+quizQuestionColumns+` FROM quiz_questions
		WHERE course_id = $1 AND id = ANY($2) ORDER BY sort_order, id`, courseID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.QuizQuestion
	for rows.Next() {
		q, err := scanQuizQuestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// QuizLearner identifies who is taking a quiz. UserID is nil for an admin
// previewing their own course, who is identified by Email and not capped.
type QuizLearner struct {
	UserID  *int64
	IsAdmin bool
	Name    string
	Email   string
}

// openAttemptSQL matches the learner's un-submitted attempt on a course
// ($1 = course id, $2 = user id or admin email).
func openAttemptSQL(who QuizLearner) (string, any) {
	if who.UserID != nil {
		return `course_id = $1 AND user_id = $2 AND submitted_at IS NULL`, *who.UserID
	}
	return `course_id = $1 AND user_id IS NULL AND is_admin AND email = $2 AND submitted_at IS NULL`, who.Email
}

func isCapViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "23514" || pgErr.Code == "23505")
}

// OpenAttempt returns the learner's started-but-unsubmitted attempt, if any.
func (r *QuizAttemptRepo) OpenAttempt(ctx context.Context, courseID int64, who QuizLearner) (models.QuizAttempt, error) {
	where, arg := openAttemptSQL(who)
	a, err := scanQuizAttempt(r.db.QueryRowContext(ctx,
		`SELECT `+quizAttemptColumns+` FROM quiz_attempts WHERE `+where+` ORDER BY id DESC LIMIT 1`, courseID, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return models.QuizAttempt{}, ErrNotFound
	}
	return a, err
}

// Usage reports how many attempts the learner has started on the course and
// how many they are entitled to (5 + admin grant). Admin previews are
// uncapped: limit is 0 and the caller treats that as unlimited.
func (r *QuizAttemptRepo) Usage(ctx context.Context, courseID int64, who QuizLearner) (used, limit int, err error) {
	if who.UserID == nil {
		err = r.db.QueryRowContext(ctx, `SELECT count(*) FROM quiz_attempts
			WHERE course_id = $1 AND user_id IS NULL AND is_admin AND email = $2`, courseID, who.Email).Scan(&used)
		return used, 0, err
	}
	err = r.db.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT max(attempt_no) FROM quiz_attempts WHERE user_id = $1 AND course_id = $2), 0),
		       $3::int + COALESCE((SELECT extra FROM quiz_attempt_grants WHERE user_id = $1 AND course_id = $2), 0)`,
		*who.UserID, courseID, models.QuizMaxAttempts).Scan(&used, &limit)
	return used, limit, err
}

// Start begins a new attempt, or returns the learner's open one (resumed =
// true; it was already counted when it started). The attempt is counted at
// start, so abandoning it still uses it up. pick chooses the question ids
// for the attempt number and runs inside the transaction. Concurrent starts
// serialise on a per-(user, course) advisory lock, and the table's unique
// constraint plus cap trigger make exceeding the limit impossible regardless
// of this code; ErrNoAttemptsLeft is returned instead of the raw DB error.
func (r *QuizAttemptRepo) Start(ctx context.Context, courseID int64, who QuizLearner,
	pick func(attemptNo int) (setNo int, ids []int64)) (a models.QuizAttempt, resumed bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return a, false, err
	}
	defer tx.Rollback()

	if who.UserID != nil {
		_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('quiz_attempt:' || $1::bigint || ':' || $2::bigint, 0))`,
			*who.UserID, courseID)
	} else {
		_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('quiz_admin:' || $1::text || ':' || $2::bigint, 0))`,
			who.Email, courseID)
	}
	if err != nil {
		return a, false, err
	}

	where, arg := openAttemptSQL(who)
	open, oerr := scanQuizAttempt(tx.QueryRowContext(ctx,
		`SELECT `+quizAttemptColumns+` FROM quiz_attempts WHERE `+where+` ORDER BY id DESC LIMIT 1`, courseID, arg))
	if oerr == nil {
		return open, true, nil
	}
	if !errors.Is(oerr, sql.ErrNoRows) {
		return a, false, oerr
	}

	var used int
	var uid, attemptNoArg any
	if who.UserID != nil {
		uid = *who.UserID
		var extra int
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(attempt_no), 0) FROM quiz_attempts WHERE user_id = $1 AND course_id = $2`,
			*who.UserID, courseID).Scan(&used); err != nil {
			return a, false, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT extra FROM quiz_attempt_grants WHERE user_id = $1 AND course_id = $2), 0)`,
			*who.UserID, courseID).Scan(&extra); err != nil {
			return a, false, err
		}
		if used >= models.QuizMaxAttempts+extra {
			return a, false, ErrNoAttemptsLeft
		}
		attemptNoArg = used + 1
	} else if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM quiz_attempts WHERE course_id = $1 AND user_id IS NULL AND is_admin AND email = $2`,
		courseID, who.Email).Scan(&used); err != nil {
		return a, false, err
	}

	setNo, ids := pick(used + 1)
	if len(ids) == 0 {
		return a, false, ErrNotFound
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO quiz_attempts (course_id, user_id, is_admin, name, email, total_questions, correct_answers,
			score_percent, passed, attempt_no, set_no, question_ids)
		VALUES ($1,$2,$3,$4,$5,$6,0,0,FALSE,$7,$8,$9) RETURNING id`,
		courseID, uid, who.IsAdmin, who.Name, who.Email, len(ids), attemptNoArg, setNo, pq.Array(ids)).Scan(&id)
	if err != nil {
		if isCapViolation(err) {
			return a, false, ErrNoAttemptsLeft
		}
		return a, false, err
	}
	if err = tx.Commit(); err != nil {
		return a, false, err
	}
	a, err = r.GetByID(ctx, id)
	return a, false, err
}

func (r *QuizAttemptRepo) GetByID(ctx context.Context, id int64) (models.QuizAttempt, error) {
	a, err := scanQuizAttempt(r.db.QueryRowContext(ctx, `SELECT `+quizAttemptColumns+` FROM quiz_attempts WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.QuizAttempt{}, ErrNotFound
	}
	return a, err
}

// Submit grades the learner's open attempt against the question ids stored
// when it started (the client only supplies chosen letters) and marks it
// submitted. The row is locked FOR UPDATE, so a double submit grades once;
// the second gets ErrAlreadySubmitted. certCode is stored only on a pass.
func (r *QuizAttemptRepo) Submit(ctx context.Context, courseID int64, who QuizLearner,
	answers map[int64]string, certCode string) (models.QuizAttempt, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return models.QuizAttempt{}, err
	}
	defer tx.Rollback()

	where, arg := openAttemptSQL(who)
	a, err := scanQuizAttempt(tx.QueryRowContext(ctx,
		`SELECT `+quizAttemptColumns+` FROM quiz_attempts WHERE `+where+` ORDER BY id DESC LIMIT 1 FOR UPDATE`, courseID, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return models.QuizAttempt{}, ErrAlreadySubmitted
	}
	if err != nil {
		return models.QuizAttempt{}, err
	}
	// FOR UPDATE re-evaluates the WHERE after waiting on a concurrent
	// submit, so a row already graded is not returned; keep the check anyway.
	if a.SubmittedAt != nil {
		return models.QuizAttempt{}, ErrAlreadySubmitted
	}

	rows, err := tx.QueryContext(ctx, `SELECT `+quizQuestionColumns+` FROM quiz_questions
		WHERE course_id = $1 AND id = ANY($2)`, courseID, pq.Array(a.QuestionIDs))
	if err != nil {
		return models.QuizAttempt{}, err
	}
	var qs []models.QuizQuestion
	for rows.Next() {
		q, err := scanQuizQuestion(rows)
		if err != nil {
			rows.Close()
			return models.QuizAttempt{}, err
		}
		qs = append(qs, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return models.QuizAttempt{}, err
	}

	correct, total, percent, passed := models.ScoreQuiz(qs, answers)
	var cert any
	if passed && certCode != "" {
		cert = certCode
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE quiz_attempts SET total_questions=$1, correct_answers=$2, score_percent=$3, passed=$4,
			certificate_code=$5, submitted_at=now() WHERE id=$6`,
		total, correct, percent, passed, cert, a.ID); err != nil {
		return models.QuizAttempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.QuizAttempt{}, err
	}
	return r.GetByID(ctx, a.ID)
}

// UsedByUser returns attempts started per course for one visitor.
func (r *QuizAttemptRepo) UsedByUser(ctx context.Context, userID int64) (map[int64]int, error) {
	return r.countMap(ctx, `SELECT course_id, COALESCE(max(attempt_no),0) FROM quiz_attempts
		WHERE user_id = $1 GROUP BY course_id`, userID)
}

func (r *QuizAttemptRepo) countMap(ctx context.Context, q string, args ...any) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var c int64
		var n int
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		out[c] = n
	}
	return out, rows.Err()
}

// QuizGrantRepo stores admin-granted extra attempts per learner and course.
type QuizGrantRepo struct{ db *sql.DB }

func NewQuizGrantRepo(db *sql.DB) *QuizGrantRepo { return &QuizGrantRepo{db: db} }

// Set records the total number of extra attempts (beyond the base 5) the
// learner may take on the course; 0 removes the grant.
func (r *QuizGrantRepo) Set(ctx context.Context, userID, courseID int64, extra int) error {
	if extra <= 0 {
		_, err := r.db.ExecContext(ctx, `DELETE FROM quiz_attempt_grants WHERE user_id = $1 AND course_id = $2`, userID, courseID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO quiz_attempt_grants (user_id, course_id, extra) VALUES ($1,$2,$3)
		ON CONFLICT (user_id, course_id) DO UPDATE SET extra = EXCLUDED.extra, updated_at = now()`, userID, courseID, extra)
	return err
}

// ListByUser returns course id -> extra attempts granted.
func (r *QuizGrantRepo) ListByUser(ctx context.Context, userID int64) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT course_id, extra FROM quiz_attempt_grants WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var c int64
		var n int
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		out[c] = n
	}
	return out, rows.Err()
}
