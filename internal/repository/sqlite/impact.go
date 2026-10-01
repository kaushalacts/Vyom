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

func (d *DB) UpsertImpactBatch(ctx context.Context, impacts []models.AssetImpact) error {
	if len(impacts) == 0 {
		return nil
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "UpsertImpacts:begin", providererr.KindInternal)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO impacts (id, asset_id, url, storage_key, requests, current_bytes, projected_bytes,
			potential_savings, savings_percent, priority_score, delivered_share, savings_share,
			request_share, estimated_optimized_size, class, reasons_json, scan_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asset_id, scan_id) DO UPDATE SET
			url = excluded.url,
			storage_key = excluded.storage_key,
			requests = excluded.requests,
			current_bytes = excluded.current_bytes,
			projected_bytes = excluded.projected_bytes,
			potential_savings = excluded.potential_savings,
			savings_percent = excluded.savings_percent,
			priority_score = excluded.priority_score,
			delivered_share = excluded.delivered_share,
			savings_share = excluded.savings_share,
			request_share = excluded.request_share,
			estimated_optimized_size = excluded.estimated_optimized_size,
			class = excluded.class,
			reasons_json = excluded.reasons_json
	`)
	if err != nil {
		_ = tx.Rollback()
		return providererr.Wrap(err, "sqlite", "UpsertImpacts:prepare", providererr.KindInternal)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, i := range impacts {
		id := models.NewID(models.PrefixImpact)
		reasonsJSON, _ := json.Marshal(i.Reasons)
		_, err := stmt.ExecContext(ctx, id, i.AssetID, i.URL, i.StorageKey, i.Requests,
			i.CurrentBytes, i.ProjectedBytes, i.PotentialSavings, i.SavingsPercent,
			i.PriorityScore, i.DeliveredShare, i.SavingsShare, i.RequestShare,
			i.EstimatedOptimizedSize, i.Class, string(reasonsJSON), i.ScanID, now)
		if err != nil {
			_ = tx.Rollback()
			return providererr.Wrap(err, "sqlite", "UpsertImpacts:exec", providererr.KindInternal)
		}
	}
	return tx.Commit()
}

func (d *DB) ListImpacts(ctx context.Context, p models.PageRequest) (*models.ImpactPage, error) {
	p = p.Normalize()
	var total int64
	if err := d.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM impacts").Scan(&total); err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListImpacts:count", providererr.KindInternal)
	}

	orderBy := "priority_score DESC"
	if p.Sort != "" {
		orderBy = p.Sort + " " + string(p.Order)
	}
	rows, err := d.q.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, asset_id, url, storage_key, requests, current_bytes, projected_bytes,
			potential_savings, savings_percent, priority_score, delivered_share, savings_share,
			request_share, estimated_optimized_size, class, reasons_json, scan_id
		FROM impacts
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, orderBy), p.Limit, p.Offset)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListImpacts:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.AssetImpact
	for rows.Next() {
		i, err := scanImpact(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *i)
	}
	return &models.ImpactPage{Items: items, Page: models.NewPageInfo(p, len(items), total)}, nil
}

func (d *DB) TopImpacts(ctx context.Context, limit int) ([]models.AssetImpact, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.q.QueryContext(ctx, `
		SELECT id, asset_id, url, storage_key, requests, current_bytes, projected_bytes,
			potential_savings, savings_percent, priority_score, delivered_share, savings_share,
			request_share, estimated_optimized_size, class, reasons_json, scan_id
		FROM impacts
		ORDER BY priority_score DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "TopImpacts:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.AssetImpact
	for rows.Next() {
		i, err := scanImpact(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *i)
	}
	return items, nil
}

func (d *DB) ImpactsByAsset(ctx context.Context, assetID string) ([]models.AssetImpact, error) {
	rows, err := d.q.QueryContext(ctx, `
		SELECT id, asset_id, url, storage_key, requests, current_bytes, projected_bytes,
			potential_savings, savings_percent, priority_score, delivered_share, savings_share,
			request_share, estimated_optimized_size, class, reasons_json, scan_id
		FROM impacts WHERE asset_id = ?
		ORDER BY priority_score DESC
	`, assetID)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ImpactsByAsset:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.AssetImpact
	for rows.Next() {
		i, err := scanImpact(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *i)
	}
	return items, nil
}

func (d *DB) DeleteImpactsByScan(ctx context.Context, scanID string) (int64, error) {
	res, err := d.q.ExecContext(ctx, "DELETE FROM impacts WHERE scan_id = ?", scanID)
	if err != nil {
		return 0, providererr.Wrap(err, "sqlite", "DeleteImpactsByScan", providererr.KindInternal)
	}
	return res.RowsAffected()
}

func scanImpact(scanner interface {
	Scan(dest ...any) error
}) (*models.AssetImpact, error) {
	var i models.AssetImpact
	var reasonsJSON string
	err := scanner.Scan(
		&i.ID, &i.AssetID, &i.URL, &i.StorageKey, &i.Requests,
		&i.CurrentBytes, &i.ProjectedBytes, &i.PotentialSavings,
		&i.SavingsPercent, &i.PriorityScore, &i.DeliveredShare,
		&i.SavingsShare, &i.RequestShare, &i.EstimatedOptimizedSize,
		&i.Class, &reasonsJSON, &i.ScanID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, providererr.NotFound("sqlite", "GetImpact", "")
		}
		return nil, providererr.Wrap(err, "sqlite", "ScanImpact", providererr.KindMalformedResponse)
	}
	if reasonsJSON != "" {
		_ = json.Unmarshal([]byte(reasonsJSON), &i.Reasons)
	}
	return &i, nil
}