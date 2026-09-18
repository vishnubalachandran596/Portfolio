package repo

import (
	"context"
	"database/sql"
)

type CourseUpdateRepo struct{ db *sql.DB }

func NewCourseUpdateRepo(db *sql.DB) *CourseUpdateRepo { return &CourseUpdateRepo{db: db} }

// Create records that a course changed. A database trigger (see migration
// 026_course_updates.sql) fans this out to every paid buyer as an
// admin_messages row — this insert is the whole notification.
func (r *CourseUpdateRepo) Create(ctx context.Context, courseID int64, note string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO course_updates (course_id, note) VALUES ($1, $2)`, courseID, note)
	return err
}
