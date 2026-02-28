package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ThreadReport struct {
	ID         uuid.UUID
	ThreadID   uuid.UUID
	ReporterID uuid.UUID
	Categories []string
	Feedback   *string
	CreatedAt  time.Time
}

type ThreadReportRow struct {
	ThreadReport
	ReporterEmail string
}

type ThreadReportRepository struct {
	db Querier
}

func NewThreadReportRepository(db Querier) (*ThreadReportRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &ThreadReportRepository{db: db}, nil
}

func (r *ThreadReportRepository) Create(
	ctx context.Context,
	threadID uuid.UUID,
	reporterID uuid.UUID,
	categories []string,
	feedback *string,
) (*ThreadReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if threadID == uuid.Nil {
		return nil, fmt.Errorf("thread_id must not be empty")
	}
	if reporterID == uuid.Nil {
		return nil, fmt.Errorf("reporter_id must not be empty")
	}
	if len(categories) == 0 {
		return nil, fmt.Errorf("categories must not be empty")
	}

	var report ThreadReport
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO thread_reports (thread_id, reporter_id, categories, feedback)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, thread_id, reporter_id, categories, feedback, created_at`,
		threadID, reporterID, categories, feedback,
	).Scan(
		&report.ID, &report.ThreadID, &report.ReporterID,
		&report.Categories, &report.Feedback, &report.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (r *ThreadReportRepository) List(
	ctx context.Context,
	limit int,
	offset int,
) ([]ThreadReportRow, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var total int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM thread_reports`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT r.id, r.thread_id, r.reporter_id, r.categories, r.feedback, r.created_at,
		        COALESCE(u.email, '') AS reporter_email
		 FROM thread_reports r
		 LEFT JOIN users u ON u.id = r.reporter_id
		 ORDER BY r.created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []ThreadReportRow
	for rows.Next() {
		var row ThreadReportRow
		if err := rows.Scan(
			&row.ID, &row.ThreadID, &row.ReporterID,
			&row.Categories, &row.Feedback, &row.CreatedAt,
			&row.ReporterEmail,
		); err != nil {
			return nil, 0, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return result, total, nil
}
