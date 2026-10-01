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

func (d *DB) CreateOptimizationJob(ctx context.Context, job *models.OptimizationJob) error {
	if job.ID == "" {
		job.ID = models.NewID(models.PrefixOptimizationJob)
	}
	countsJSON, _ := json.Marshal(job.Counts)
	_, err := d.q.ExecContext(ctx, `
		INSERT INTO optimization_jobs (id, dry_run, status, policy_name, scan_id, counts_json,
			started_at, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, job.ID, job.DryRun, job.Status, job.Policy, job.ScanID,
		string(countsJSON), job.StartedAt.UTC().Format(time.RFC3339), job.Error)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "CreateOptimizationJob", providererr.KindInternal)
	}
	return nil
}

func (d *DB) UpdateOptimizationJob(ctx context.Context, job *models.OptimizationJob) error {
	countsJSON, _ := json.Marshal(job.Counts)
	var finishedAt any
	if job.FinishedAt != nil {
		finishedAt = job.FinishedAt.UTC().Format(time.RFC3339)
	}
	_, err := d.q.ExecContext(ctx, `
		UPDATE optimization_jobs SET dry_run = ?, status = ?, policy_name = ?, scan_id = ?,
			counts_json = ?, finished_at = ?, error = ?
		WHERE id = ?
	`, job.DryRun, job.Status, job.Policy, job.ScanID,
		string(countsJSON), finishedAt, job.Error, job.ID)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "UpdateOptimizationJob", providererr.KindInternal)
	}
	return nil
}

func (d *DB) GetOptimizationJob(ctx context.Context, id string) (*models.OptimizationJob, error) {
	row := d.q.QueryRowContext(ctx, `
		SELECT id, dry_run, status, policy_name, scan_id, counts_json, started_at, finished_at, error
		FROM optimization_jobs WHERE id = ?
	`, id)
	return scanOptJob(row)
}

func (d *DB) ListOptimizationJobs(ctx context.Context, p models.PageRequest) (*models.OptimizationJobPage, error) {
	p = p.Normalize()
	var total int64
	if err := d.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM optimization_jobs").Scan(&total); err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListOptimizationJobs:count", providererr.KindInternal)
	}

	orderBy := "started_at DESC"
	if p.Sort != "" {
		orderBy = p.Sort + " " + string(p.Order)
	}
	rows, err := d.q.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, dry_run, status, policy_name, scan_id, counts_json, started_at, finished_at, error
		FROM optimization_jobs
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, orderBy), p.Limit, p.Offset)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListOptimizationJobs:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.OptimizationJob
	for rows.Next() {
		j, err := scanOptJob(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *j)
	}
	return &models.OptimizationJobPage{Items: items, Page: models.NewPageInfo(p, len(items), total)}, nil
}

func (d *DB) AppendManifest(ctx context.Context, entries []models.ManifestEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "AppendManifest:begin", providererr.KindInternal)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO manifest_entries (id, job_id, asset, original_size, optimized_size,
			original_format, optimized_format, ssim, psnr, original_key, optimized_key,
			original_hash, optimized_hash, requests, bytes_delivered, estimated_savings_bytes,
			timestamp, operation, status, dry_run, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return providererr.Wrap(err, "sqlite", "AppendManifest:prepare", providererr.KindInternal)
	}
	defer stmt.Close()

	for _, e := range entries {
		id := models.NewID(models.PrefixOptimizationResult)
		_, err := stmt.ExecContext(ctx, id, e.JobID, e.Asset, e.OriginalSize, e.OptimizedSize,
			e.OriginalFormat, e.OptimizedFormat, e.SSIM, e.PSNR, e.OriginalKey,
			e.OptimizedKey, e.OriginalHash, e.OptimizedHash, e.Requests, e.BytesDelivered,
			e.EstimatedSavingsBytes, e.Timestamp.UTC().Format(time.RFC3339), e.Operation,
			e.Status, e.DryRun, e.Reason)
		if err != nil {
			_ = tx.Rollback()
			return providererr.Wrap(err, "sqlite", "AppendManifest:exec", providererr.KindInternal)
		}
	}
	return tx.Commit()
}

func (d *DB) GetManifest(ctx context.Context, jobID string) ([]models.ManifestEntry, error) {
	rows, err := d.q.QueryContext(ctx, `
		SELECT id, job_id, asset, original_size, optimized_size, original_format, optimized_format,
			ssim, psnr, original_key, optimized_key, original_hash, optimized_hash,
			requests, bytes_delivered, estimated_savings_bytes, timestamp, operation, status, dry_run, reason
		FROM manifest_entries WHERE job_id = ?
		ORDER BY timestamp
	`, jobID)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "Manifest:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.ManifestEntry
	for rows.Next() {
		var e models.ManifestEntry
		var ts string
		if err := rows.Scan(
			&e.ID, &e.JobID, &e.Asset, &e.OriginalSize, &e.OptimizedSize,
			&e.OriginalFormat, &e.OptimizedFormat, &e.SSIM, &e.PSNR,
			&e.OriginalKey, &e.OptimizedKey, &e.OriginalHash, &e.OptimizedHash,
			&e.Requests, &e.BytesDelivered, &e.EstimatedSavingsBytes,
			&ts, &e.Operation, &e.Status, &e.DryRun, &e.Reason,
		); err != nil {
			return nil, providererr.Wrap(err, "sqlite", "Manifest:scan", providererr.KindMalformedResponse)
		}
		e.Timestamp, _ = time.Parse(time.RFC3339, ts)
		items = append(items, e)
	}
	return items, nil
}

func scanOptJob(scanner interface {
	Scan(dest ...any) error
}) (*models.OptimizationJob, error) {
	var j models.OptimizationJob
	var countsJSON, startedAt, finishedAtStr string
	var finishedAt sql.NullString
	err := scanner.Scan(
		&j.ID, &j.DryRun, &j.Status, &j.PolicyName, &j.ScanID,
		&countsJSON, &startedAt, &finishedAtStr, &j.Error,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, providererr.NotFound("sqlite", "GetOptimizationJob", "")
		}
		return nil, providererr.Wrap(err, "sqlite", "ScanOptimizationJob", providererr.KindMalformedResponse)
	}
	_ = json.Unmarshal([]byte(countsJSON), &j.Counts)
	j.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if finishedAtStr != "" {
		t, _ := time.Parse(time.RFC3339, finishedAtStr)
		j.FinishedAt = &t
	}
	return &j, nil
}