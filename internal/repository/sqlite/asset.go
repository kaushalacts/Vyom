package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

// AssetRepo implementation

func (d *DB) UpsertAsset(ctx context.Context, a *models.Asset) error {
	if a.ID == "" {
		a.ID = models.NewID(models.PrefixAsset)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.q.ExecContext(ctx, `
		INSERT INTO assets (id, url, storage_key, storage_provider, cdn_provider, content_type,
			format, size_bytes, width, height, requests, bytes_delivered, first_seen, last_seen,
			last_modified, etag, content_hash, scan_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			url = excluded.url,
			storage_key = excluded.storage_key,
			storage_provider = excluded.storage_provider,
			cdn_provider = excluded.cdn_provider,
			content_type = excluded.content_type,
			format = excluded.format,
			size_bytes = excluded.size_bytes,
			width = excluded.width,
			height = excluded.height,
			requests = excluded.requests,
			bytes_delivered = excluded.bytes_delivered,
			first_seen = excluded.first_seen,
			last_seen = excluded.last_seen,
			last_modified = excluded.last_modified,
			etag = excluded.etag,
			content_hash = excluded.content_hash,
			scan_id = excluded.scan_id,
			updated_at = excluded.updated_at
	`, a.ID, a.URL, a.StorageKey, a.StorageProvider, a.CDNProvider, a.ContentType,
		a.Format, a.SizeBytes, a.Width, a.Height, a.Requests, a.BytesDelivered,
		a.FirstSeen.UTC().Format(time.RFC3339), a.LastSeen.UTC().Format(time.RFC3339),
		nullTime(a.LastModified), a.ETag, a.ContentHash, a.ScanID, now, now)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "UpsertAsset", providererr.KindInternal)
	}
	return nil
}

func (d *DB) GetAsset(ctx context.Context, id string) (*models.Asset, error) {
	row := d.q.QueryRowContext(ctx, `
		SELECT id, url, storage_key, storage_provider, cdn_provider, content_type, format,
			size_bytes, width, height, requests, bytes_delivered, first_seen, last_seen,
			last_modified, etag, content_hash, scan_id
		FROM assets WHERE id = ?
	`, id)
	return scanAsset(row)
}

func (d *DB) ListAssets(ctx context.Context, f models.AssetFilter, p models.PageRequest) (*models.AssetPage, error) {
	p = p.Normalize()
	where, args := buildAssetWhere(f)
	countQuery := "SELECT COUNT(*) FROM assets" + where
	var total int64
	if err := d.q.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListAssets:count", providererr.KindInternal)
	}

	orderBy := "bytes_delivered DESC"
	if p.Sort != "" {
		orderBy = p.Sort + " " + string(p.Order)
	}
	query := fmt.Sprintf(`
		SELECT id, url, storage_key, storage_provider, cdn_provider, content_type, format,
			size_bytes, width, height, requests, bytes_delivered, first_seen, last_seen,
			last_modified, etag, content_hash, scan_id
		FROM assets %s
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, where, orderBy)
	args = append(args, p.Limit, p.Offset)

	rows, err := d.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListAssets:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *a)
	}
	return &models.AssetPage{Items: items, Page: models.NewPageInfo(p, len(items), total)}, nil
}

func (d *DB) CountAssets(ctx context.Context, f models.AssetFilter) (int64, error) {
	where, args := buildAssetWhere(f)
	var total int64
	err := d.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets"+where, args...).Scan(&total)
	if err != nil {
		return 0, providererr.Wrap(err, "sqlite", "CountAssets", providererr.KindInternal)
	}
	return total, nil
}

func (d *DB) DeleteAsset(ctx context.Context, id string) error {
	_, err := d.q.ExecContext(ctx, "DELETE FROM assets WHERE id = ?", id)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "DeleteAsset", providererr.KindInternal)
	}
	return nil
}

func (d *DB) DeleteAssetsByScan(ctx context.Context, scanID string) (int64, error) {
	res, err := d.q.ExecContext(ctx, "DELETE FROM assets WHERE scan_id = ?", scanID)
	if err != nil {
		return 0, providererr.Wrap(err, "sqlite", "DeleteAssetsByScan", providererr.KindInternal)
	}
	return res.RowsAffected()
}

func scanAsset(scanner interface {
	Scan(dest ...any) error
}) (*models.Asset, error) {
	var a models.Asset
	var firstSeen, lastSeen, lastModified sql.NullString
	err := scanner.Scan(
		&a.ID, &a.URL, &a.StorageKey, &a.StorageProvider, &a.CDNProvider,
		&a.ContentType, &a.Format, &a.SizeBytes, &a.Width, &a.Height,
		&a.Requests, &a.BytesDelivered, &firstSeen, &lastSeen,
		&lastModified, &a.ETag, &a.ContentHash, &a.ScanID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, providererr.NotFound("sqlite", "GetAsset", "")
		}
		return nil, providererr.Wrap(err, "sqlite", "ScanAsset", providererr.KindMalformedResponse)
	}
	a.FirstSeen, _ = time.Parse(time.RFC3339, firstSeen.String)
	a.LastSeen, _ = time.Parse(time.RFC3339, lastSeen.String)
	if lastModified.Valid {
		a.LastModified, _ = time.Parse(time.RFC3339, lastModified.String)
	}
	return &a, nil
}

func buildAssetWhere(f models.AssetFilter) (string, []any) {
	var clauses []string
	var args []any
	if f.ScanID != "" {
		clauses = append(clauses, "scan_id = ?")
		args = append(args, f.ScanID)
	}
	if f.CDNProvider != "" {
		clauses = append(clauses, "cdn_provider = ?")
		args = append(args, f.CDNProvider)
	}
	if f.StorageProvider != "" {
		clauses = append(clauses, "storage_provider = ?")
		args = append(args, f.StorageProvider)
	}
	if f.Format != "" {
		clauses = append(clauses, "format = ?")
		args = append(args, f.Format)
	}
	if f.ContentType != "" {
		clauses = append(clauses, "content_type = ?")
		args = append(args, f.ContentType)
	}
	if f.Kind != "" {
		// Join not needed; filter in application if necessary
	}
	if f.MinSizeBytes > 0 {
		clauses = append(clauses, "size_bytes >= ?")
		args = append(args, f.MinSizeBytes)
	}
	if f.MinBytesDelivered > 0 {
		clauses = append(clauses, "bytes_delivered >= ?")
		args = append(args, f.MinBytesDelivered)
	}
	if f.MinRequests > 0 {
		clauses = append(clauses, "requests >= ?")
		args = append(args, f.MinRequests)
	}
	if f.Search != "" {
		clauses = append(clauses, "(url LIKE ? OR storage_key LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like)
	}
	if !f.Since.IsZero() {
		clauses = append(clauses, "last_seen >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	return where, args
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}