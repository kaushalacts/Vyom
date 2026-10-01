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

// TrafficRepo implementation

func (d *DB) UpsertTrafficBatch(ctx context.Context, records []models.TrafficRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "UpsertTraffic:begin", providererr.KindInternal)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO traffic_records (id, url, requests, bytes_delivered, start_time, end_time,
			content_type, format, status_code, country, provider, scan_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(url, start_time, end_time, provider) DO UPDATE SET
			requests = excluded.requests,
			bytes_delivered = excluded.bytes_delivered,
			content_type = excluded.content_type,
			format = excluded.format,
			status_code = excluded.status_code,
			country = excluded.country
	`)
	if err != nil {
		_ = tx.Rollback()
		return providererr.Wrap(err, "sqlite", "UpsertTraffic:prepare", providererr.KindInternal)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range records {
		id := models.NewID(models.PrefixTrafficRecord)
		_, err := stmt.ExecContext(ctx, id, r.URL, r.Requests, r.BytesDelivered,
			r.StartTime.UTC().Format(time.RFC3339), r.EndTime.UTC().Format(time.RFC3339),
			r.ContentType, r.Format, r.StatusCode, r.Country, r.Provider, r.ScanID, now)
		if err != nil {
			_ = tx.Rollback()
			return providererr.Wrap(err, "sqlite", "UpsertTraffic:exec", providererr.KindInternal)
		}
	}
	return tx.Commit()
}

func (d *DB) ListTraffic(ctx context.Context, p models.PageRequest) (*models.TrafficPage, error) {
	p = p.Normalize()
	var total int64
	if err := d.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM traffic_records").Scan(&total); err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListTraffic:count", providererr.KindInternal)
	}

	orderBy := "bytes_delivered DESC"
	if p.Sort != "" {
		orderBy = p.Sort + " " + string(p.Order)
	}
	rows, err := d.q.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, url, requests, bytes_delivered, start_time, end_time, content_type, format,
			status_code, country, provider, scan_id
		FROM traffic_records
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, orderBy), p.Limit, p.Offset)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListTraffic:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.TrafficRecord
	for rows.Next() {
		t, err := scanTraffic(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *t)
	}
	return &models.TrafficPage{Items: items, Page: models.NewPageInfo(p, len(items), total)}, nil
}

func (d *DB) CountTraffic(ctx context.Context) (int64, error) {
	var total int64
	err := d.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM traffic_records").Scan(&total)
	if err != nil {
		return 0, providererr.Wrap(err, "sqlite", "CountTraffic", providererr.KindInternal)
	}
	return total, nil
}

func (d *DB) DeleteTrafficByWindow(ctx context.Context, start, end time.Time) (int64, error) {
	res, err := d.q.ExecContext(ctx,
		"DELETE FROM traffic_records WHERE start_time >= ? AND end_time <= ?",
		start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, providererr.Wrap(err, "sqlite", "DeleteTrafficByWindow", providererr.KindInternal)
	}
	return res.RowsAffected()
}

func scanTraffic(scanner interface {
	Scan(dest ...any) error
}) (*models.TrafficRecord, error) {
	var t models.TrafficRecord
	var startTime, endTime string
	err := scanner.Scan(
		&t.ID, &t.URL, &t.Requests, &t.BytesDelivered,
		&startTime, &endTime, &t.ContentType, &t.Format,
		&t.StatusCode, &t.Country, &t.Provider, &t.ScanID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, providererr.NotFound("sqlite", "GetTraffic", "")
		}
		return nil, providererr.Wrap(err, "sqlite", "ScanTraffic", providererr.KindMalformedResponse)
	}
	t.StartTime, _ = time.Parse(time.RFC3339, startTime)
	t.EndTime, _ = time.Parse(time.RFC3339, endTime)
	return &t, nil
}