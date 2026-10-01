package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func (d *DB) CreateScan(ctx context.Context, job *models.ScanJob) error {
	if job.ID == "" {
		job.ID = models.NewID(models.PrefixScanJob)
	}
	countsJSON, _ := json.Marshal(job.Counts)
	warningsJSON, _ := json.Marshal(job.Warnings)
	_, err := d.q.ExecContext(ctx, `
		INSERT INTO scans (id, status, current_stage, progress_pct, window_start, window_end,
			cdn_provider, storage_provider, policy_name, counts_json, warnings_json,
			error, source, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, job.ID, job.Status, job.CurrentStage, job.ProgressPct,
		job.Window.Start.UTC().Format(time.RFC3339), job.Window.End.UTC().Format(time.RFC3339),
		job.CDNProvider, job.StorageProvider, job.PolicyName,
		string(countsJSON), string(warningsJSON), job.Error, job.Source,
		job.StartedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return providererr.Wrap(err, "sqlite", "CreateScan", providererr.KindInternal)
	}
	return nil
}

func (d *DB) UpdateScan(ctx context.Context, job *models.ScanJob) error {
	countsJSON, _ := json.Marshal(job.Counts)
	warningsJSON, _ := json.Marshal(job.Warnings)
	var finishedAt any
	if job.FinishedAt != nil {
		finishedAt = job.FinishedAt.UTC().Format(time.RFC3339)
	}
	_, err := d.q.ExecContext(ctx, `
		UPDATE scans SET status = ?, current_stage = ?, progress_pct = ?,
			counts_json = ?, warnings_json = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, job.Status, job.CurrentStage, job.ProgressPct,
		string(countsJSON), string(warningsJSON), job.Error, finishedAt, job.ID)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "UpdateScan", providererr.KindInternal)
	}
	return nil
}

func (d *DB) GetScan(ctx context.Context, id string) (*models.ScanJob, error) {
	row := d.q.QueryRowContext(ctx, `
		SELECT id, status, current_stage, progress_pct, window_start, window_end,
			cdn_provider, storage_provider, policy_name, counts_json, warnings_json,
			error, source, started_at, finished_at
		FROM scans WHERE id = ?
	`, id)
	return scanScanJob(row)
}

func (d *DB) ListScans(ctx context.Context, p models.PageRequest) (*models.ScanPage, error) {
	p = p.Normalize()
	var total int64
	if err := d.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM scans").Scan(&total); err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListScans:count", providererr.KindInternal)
	}

	orderBy := "started_at DESC"
	if p.Sort != "" {
		orderBy = p.Sort + " " + string(p.Order)
	}
	rows, err := d.q.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, status, current_stage, progress_pct, window_start, window_end,
			cdn_provider, storage_provider, policy_name, counts_json, warnings_json,
			error, source, started_at, finished_at
		FROM scans
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, orderBy), p.Limit, p.Offset)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListScans:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.ScanJob
	for rows.Next() {
		s, err := scanScanJob(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *s)
	}
	return &models.ScanPage{Items: items, Page: models.NewPageInfo(p, len(items), total)}, nil
}

func (d *DB) LatestScan(ctx context.Context) (*models.ScanJob, error) {
	row := d.q.QueryRowContext(ctx, `
		SELECT id, status, current_stage, progress_pct, window_start, window_end,
			cdn_provider, storage_provider, policy_name, counts_json, warnings_json,
			error, source, started_at, finished_at
		FROM scans
		ORDER BY started_at DESC
		LIMIT 1
	`)
	return scanScanJob(row)
}

func scanScanJob(scanner interface {
	Scan(dest ...any) error
}) (*models.ScanJob, error) {
	var s models.ScanJob
	var countsJSON, warningsJSON, startedAt, finishedAtStr string
	var finishedAt sql.NullString
	err := scanner.Scan(
		&s.ID, &s.Status, &s.CurrentStage, &s.ProgressPct,
		&s.Window.Start, &s.Window.End, &s.CDNProvider, &s.StorageProvider,
		&s.PolicyName, &countsJSON, &warningsJSON, &s.Error, &s.Source,
		&startedAt, &finishedAtStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, providererr.NotFound("sqlite", "GetScan", "")
		}
		return nil, providererr.Wrap(err, "sqlite", "ScanScanJob", providererr.KindMalformedResponse)
	}
	_ = json.Unmarshal([]byte(countsJSON), &s.Counts)
	_ = json.Unmarshal([]byte(warningsJSON), &s.Warnings)
	s.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if finishedAtStr != "" {
		t, _ := time.Parse(time.RFC3339, finishedAtStr)
		s.FinishedAt = &t
	}
	return &s, nil
}