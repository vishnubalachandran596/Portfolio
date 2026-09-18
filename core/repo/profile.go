package repo

import (
	"context"
	"database/sql"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ProfileRepo struct{ db *sql.DB }

func NewProfileRepo(db *sql.DB) *ProfileRepo { return &ProfileRepo{db: db} }

func (r *ProfileRepo) Get(ctx context.Context) (models.Profile, error) {
	var p models.Profile
	err := r.db.QueryRowContext(ctx, `
		SELECT name, title, tagline, hero_subtitle, summary, location, email,
		       linkedin_url, github_url, resume_url, photo_url, years_experience, updated_at
		FROM profile WHERE id = 1`).Scan(
		&p.Name, &p.Title, &p.Tagline, &p.HeroSubtitle, &p.Summary, &p.Location, &p.Email,
		&p.LinkedInURL, &p.GitHubURL, &p.ResumeURL, &p.PhotoURL, &p.YearsExperience, &p.UpdatedAt,
	)
	return p, err
}

func (r *ProfileRepo) Update(ctx context.Context, p models.Profile) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO profile (id, name, title, tagline, hero_subtitle, summary, location, email,
		                      linkedin_url, github_url, resume_url, photo_url, years_experience, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, title = EXCLUDED.title, tagline = EXCLUDED.tagline,
			hero_subtitle = EXCLUDED.hero_subtitle, summary = EXCLUDED.summary,
			location = EXCLUDED.location, email = EXCLUDED.email,
			linkedin_url = EXCLUDED.linkedin_url, github_url = EXCLUDED.github_url,
			resume_url = EXCLUDED.resume_url, photo_url = EXCLUDED.photo_url,
			years_experience = EXCLUDED.years_experience, updated_at = now()`,
		p.Name, p.Title, p.Tagline, p.HeroSubtitle, p.Summary, p.Location, p.Email,
		p.LinkedInURL, p.GitHubURL, p.ResumeURL, p.PhotoURL, p.YearsExperience,
	)
	return err
}

type SkillGroupRepo struct{ db *sql.DB }

func NewSkillGroupRepo(db *sql.DB) *SkillGroupRepo { return &SkillGroupRepo{db: db} }

func (r *SkillGroupRepo) List(ctx context.Context) ([]models.SkillGroup, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, category, level, achievement, items, sort_order FROM skill_groups ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SkillGroup
	for rows.Next() {
		var g models.SkillGroup
		var items string
		if err := rows.Scan(&g.ID, &g.Category, &g.Level, &g.Achievement, &items, &g.SortOrder); err != nil {
			return nil, err
		}
		g.Items = splitNonEmpty(items, ",")
		out = append(out, g)
	}
	return out, rows.Err()
}

type ExperienceRepo struct{ db *sql.DB }

func NewExperienceRepo(db *sql.DB) *ExperienceRepo { return &ExperienceRepo{db: db} }

func (r *ExperienceRepo) List(ctx context.Context) ([]models.Experience, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, company, role, location, start_date, end_date, bullets, sort_order
		FROM experiences ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Experience
	for rows.Next() {
		var e models.Experience
		var bullets string
		if err := rows.Scan(&e.ID, &e.Company, &e.Role, &e.Location, &e.StartDate, &e.EndDate, &bullets, &e.SortOrder); err != nil {
			return nil, err
		}
		e.Bullets = splitNonEmpty(bullets, "\n")
		out = append(out, e)
	}
	return out, rows.Err()
}

type EducationRepo struct{ db *sql.DB }

func NewEducationRepo(db *sql.DB) *EducationRepo { return &EducationRepo{db: db} }

func (r *EducationRepo) List(ctx context.Context) ([]models.Education, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, institution, degree, field, COALESCE(start_year,0), COALESCE(end_year,0), details, sort_order
		FROM education ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Education
	for rows.Next() {
		var e models.Education
		if err := rows.Scan(&e.ID, &e.Institution, &e.Degree, &e.Field, &e.StartYear, &e.EndYear, &e.Details, &e.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type HomeFeatureRepo struct{ db *sql.DB }

func NewHomeFeatureRepo(db *sql.DB) *HomeFeatureRepo { return &HomeFeatureRepo{db: db} }

func (r *HomeFeatureRepo) List(ctx context.Context) ([]models.HomeFeature, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, title, description, sort_order FROM home_features ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.HomeFeature
	for rows.Next() {
		var f models.HomeFeature
		if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func splitNonEmpty(s, sep string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
