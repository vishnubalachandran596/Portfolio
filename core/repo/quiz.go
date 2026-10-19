package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

var (
	// ErrNoAttemptsLeft is returned by Start when the learner has used every
	// attempt they are entitled to (5 plus any admin grant).
	ErrNoAttemptsLeft = errors.New("quiz: no attempts left")
	// ErrAlreadySubmitted is returned when grading an attempt that has
	// already been graded (or does not exist for the caller).
	ErrAlreadySubmitted = errors.New("quiz: attempt already submitted")
)

type QuizQuestionRepo struct{ db *sql.DB }

func NewQuizQuestionRepo(db *sql.DB) *QuizQuestionRepo { return &QuizQuestionRepo{db: db} }

const quizQuestionColumns = `id, course_id, module, COALESCE(set_no,0), question, option_a, option_b, option_c, option_d, correct_option, sort_order`

func scanQuizQuestion(row interface{ Scan(...any) error }) (models.QuizQuestion, error) {
	var q models.QuizQuestion
	err := row.Scan(&q.ID, &q.CourseID, &q.Module, &q.SetNo, &q.Question, &q.OptionA, &q.OptionB, &q.OptionC, &q.OptionD, &q.CorrectOption, &q.SortOrder)
	return q, err
}

func (r *QuizQuestionRepo) ListByCourse(ctx context.Context, courseID int64) ([]models.QuizQuestion, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+quizQuestionColumns+` FROM quiz_questions WHERE course_id = $1 ORDER BY sort_order, id`, courseID)
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

func (r *QuizQuestionRepo) CountByCourse(ctx context.Context, courseID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM quiz_questions WHERE course_id = $1`, courseID).Scan(&n)
	return n, err
}

func (r *QuizQuestionRepo) GetByID(ctx context.Context, id int64) (models.QuizQuestion, error) {
	q, err := scanQuizQuestion(r.db.QueryRowContext(ctx, `SELECT `+quizQuestionColumns+` FROM quiz_questions WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.QuizQuestion{}, ErrNotFound
	}
	return q, err
}

func (r *QuizQuestionRepo) Create(ctx context.Context, q models.QuizQuestion) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO quiz_questions (course_id, module, question, option_a, option_b, option_c, option_d, correct_option, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		q.CourseID, q.Module, q.Question, q.OptionA, q.OptionB, q.OptionC, q.OptionD, q.CorrectOption, q.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *QuizQuestionRepo) Update(ctx context.Context, q models.QuizQuestion) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE quiz_questions SET module=$9, question=$1, option_a=$2, option_b=$3, option_c=$4, option_d=$5, correct_option=$6, sort_order=$7
		WHERE id=$8`,
		q.Question, q.OptionA, q.OptionB, q.OptionC, q.OptionD, q.CorrectOption, q.SortOrder, q.ID, q.Module,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *QuizQuestionRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM quiz_questions WHERE id = $1`, id)
	return err
}

type QuizAttemptRepo struct{ db *sql.DB }

func NewQuizAttemptRepo(db *sql.DB) *QuizAttemptRepo { return &QuizAttemptRepo{db: db} }

const quizAttemptColumns = `id, course_id, user_id, is_admin, name, email, total_questions, correct_answers, score_percent, passed, certificate_code, created_at, COALESCE(attempt_no,0), COALESCE(set_no,0), question_ids, submitted_at`

func scanQuizAttempt(row interface{ Scan(...any) error }) (models.QuizAttempt, error) {
	var a models.QuizAttempt
	var userID sql.NullInt64
	var certCode sql.NullString
	var ids pq.Int64Array
	var submitted sql.NullTime
	err := row.Scan(&a.ID, &a.CourseID, &userID, &a.IsAdmin, &a.Name, &a.Email,
		&a.TotalQuestions, &a.CorrectAnswers, &a.ScorePercent, &a.Passed, &certCode, &a.CreatedAt,
		&a.AttemptNo, &a.SetNo, &ids, &submitted)
	a.QuestionIDs = []int64(ids)
	if submitted.Valid {
		a.SubmittedAt = &submitted.Time
	}
	if userID.Valid {
		a.UserID = &userID.Int64
	}
	a.CertificateCode = certCode.String
	return a, err
}

// BestPassingAttempt returns the most recent passing attempt for a given
// course + email — the record a certificate download resolves from.
func (r *QuizAttemptRepo) BestPassingAttempt(ctx context.Context, courseID int64, email string) (models.QuizAttempt, error) {
	a, err := scanQuizAttempt(r.db.QueryRowContext(ctx, `
		SELECT `+quizAttemptColumns+` FROM quiz_attempts
		WHERE course_id = $1 AND email = $2 AND passed = TRUE
		ORDER BY created_at DESC LIMIT 1`, courseID, email))
	if errors.Is(err, sql.ErrNoRows) {
		return models.QuizAttempt{}, ErrNotFound
	}
	return a, err
}

func (r *QuizAttemptRepo) GetByCertificateCode(ctx context.Context, code string) (models.QuizAttempt, error) {
	a, err := scanQuizAttempt(r.db.QueryRowContext(ctx, `SELECT `+quizAttemptColumns+` FROM quiz_attempts WHERE certificate_code = $1`, code))
	if errors.Is(err, sql.ErrNoRows) {
		return models.QuizAttempt{}, ErrNotFound
	}
	return a, err
}

// ListByCourseAndEmail returns every attempt (pass or fail) for a given
// course + email, newest first.
func (r *QuizAttemptRepo) ListByCourseAndEmail(ctx context.Context, courseID int64, email string) ([]models.QuizAttempt, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+quizAttemptColumns+` FROM quiz_attempts
		WHERE course_id = $1 AND email = $2 ORDER BY created_at DESC`, courseID, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.QuizAttempt
	for rows.Next() {
		a, err := scanQuizAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
