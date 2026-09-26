package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ArticleContributionRepo struct{ db *sql.DB }

func NewArticleContributionRepo(db *sql.DB) *ArticleContributionRepo {
	return &ArticleContributionRepo{db: db}
}

const articleContributionColumns = `id, article_id, COALESCE(user_id, 0), email, amount_cents, currency, status, razorpay_order_id, razorpay_payment_id, created_at, paid_at`

func scanArticleContribution(row interface{ Scan(...any) error }) (models.ArticleContribution, error) {
	var c models.ArticleContribution
	var paidAt sql.NullTime
	err := row.Scan(&c.ID, &c.ArticleID, &c.UserID, &c.Email, &c.AmountCents, &c.Currency, &c.Status,
		&c.RazorpayOrderID, &c.RazorpayPaymentID, &c.CreatedAt, &paidAt)
	if paidAt.Valid {
		c.PaidAt = &paidAt.Time
	}
	return c, err
}

func (r *ArticleContributionRepo) Create(ctx context.Context, c models.ArticleContribution) (int64, error) {
	var userID any
	if c.UserID > 0 {
		userID = c.UserID
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO article_contributions (article_id, user_id, email, amount_cents, currency, status, razorpay_order_id)
		VALUES ($1, $2, $3, $4, $5, 'created', $6)
		RETURNING id`,
		c.ArticleID, userID, c.Email, c.AmountCents, c.Currency, c.RazorpayOrderID,
	).Scan(&id)
	return id, err
}

func (r *ArticleContributionRepo) GetByID(ctx context.Context, id int64) (models.ArticleContribution, error) {
	c, err := scanArticleContribution(r.db.QueryRowContext(ctx, `SELECT `+articleContributionColumns+` FROM article_contributions WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.ArticleContribution{}, ErrNotFound
	}
	return c, err
}

func (r *ArticleContributionRepo) GetByRazorpayOrderID(ctx context.Context, orderID string) (models.ArticleContribution, error) {
	c, err := scanArticleContribution(r.db.QueryRowContext(ctx, `SELECT `+articleContributionColumns+` FROM article_contributions WHERE razorpay_order_id = $1`, orderID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.ArticleContribution{}, ErrNotFound
	}
	return c, err
}

func (r *ArticleContributionRepo) MarkPaid(ctx context.Context, orderID, paymentID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE article_contributions
		SET status = 'paid', razorpay_payment_id = $1, paid_at = now()
		WHERE razorpay_order_id = $2 AND status = 'created'`,
		paymentID, orderID)
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

func (r *ArticleContributionRepo) ListPaidByUserID(ctx context.Context, userID int64) ([]models.ArticleContribution, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+articleContributionColumns+`
		FROM article_contributions
		WHERE user_id = $1 AND status = 'paid'
		ORDER BY paid_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return scanArticleContributions(rows)
}

func (r *ArticleContributionRepo) ListByUserID(ctx context.Context, userID int64) ([]models.ArticleContribution, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+articleContributionColumns+`
		FROM article_contributions
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return scanArticleContributions(rows)
}

func (r *ArticleContributionRepo) ListByArticle(ctx context.Context, articleID int64) ([]models.ArticleContribution, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+articleContributionColumns+`
		FROM article_contributions
		WHERE article_id = $1
		ORDER BY created_at DESC`, articleID)
	if err != nil {
		return nil, err
	}
	return scanArticleContributions(rows)
}

func (r *ArticleContributionRepo) ListAll(ctx context.Context) ([]models.ArticleContribution, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+articleContributionColumns+` FROM article_contributions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	return scanArticleContributions(rows)
}

func (r *ArticleContributionRepo) CountPaid(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM article_contributions WHERE status = 'paid'`).Scan(&n)
	return n, err
}

// ArticlePaidSummary is the paid-contribution rollup for one article,
// used by the admin list so the page is one GROUP BY instead of N
// lookups.
type ArticlePaidSummary struct {
	PaidCount int
	Totals    map[string]int
}

func (r *ArticleContributionRepo) SummarizePaidByArticle(ctx context.Context) (map[int64]ArticlePaidSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT article_id, currency, COUNT(*), COALESCE(SUM(amount_cents), 0)
		FROM article_contributions
		WHERE status = 'paid'
		GROUP BY article_id, currency`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]ArticlePaidSummary{}
	for rows.Next() {
		var articleID int64
		var currency string
		var count, cents int
		if err := rows.Scan(&articleID, &currency, &count, &cents); err != nil {
			return nil, err
		}
		s := out[articleID]
		if s.Totals == nil {
			s.Totals = map[string]int{}
		}
		s.PaidCount += count
		s.Totals[currency] += cents
		out[articleID] = s
	}
	return out, rows.Err()
}

func scanArticleContributions(rows *sql.Rows) ([]models.ArticleContribution, error) {
	defer rows.Close()
	var out []models.ArticleContribution
	for rows.Next() {
		c, err := scanArticleContribution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
