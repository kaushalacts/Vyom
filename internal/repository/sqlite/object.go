package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

// ObjectRepo implementation

func (d *DB) UpsertObjectBatch(ctx context.Context, objs []models.ObjectMetadata) error {
	if len(objs) == 0 {
		return nil
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "UpsertObjects:begin", providererr.KindInternal)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO objects (id, key, url, size_bytes, content_type, last_modified, etag,
			blob_type, storage_class, provider, container, scan_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key, provider, container) DO UPDATE SET
			url = excluded.url,
			size_bytes = excluded.size_bytes,
			content_type = excluded.content_type,
			last_modified = excluded.last_modified,
			etag = excluded.etag,
			blob_type = excluded.blob_type,
			storage_class = excluded.storage_class,
			scan_id = excluded.scan_id,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		_ = tx.Rollback()
		return providererr.Wrap(err, "sqlite", "UpsertObjects:prepare", providererr.KindInternal)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, o := range objs {
		id := models.NewID(models.PrefixObject)
		_, err := stmt.ExecContext(ctx, id, o.Key, o.URL, o.SizeBytes, o.ContentType,
			o.LastModified.UTC().Format(time.RFC3339), o.ETag, o.BlobType,
			o.StorageClass, o.Provider, o.Container, o.ScanID, now, now)
		if err != nil {
			_ = tx.Rollback()
			return providererr.Wrap(err, "sqlite", "UpsertObjects:exec", providererr.KindInternal)
		}
	}
	return tx.Commit()
}

func (d *DB) GetObject(ctx context.Context, key string) (*models.ObjectMetadata, error) {
	row := d.q.QueryRowContext(ctx, `
		SELECT id, key, url, size_bytes, content_type, last_modified, etag, blob_type,
			storage_class, provider, container, scan_id
		FROM objects WHERE key = ?
	`, key)
	return scanObject(row)
}

func (d *DB) ListObjects(ctx context.Context, p models.PageRequest) (*models.ObjectPage, error) {
	p = p.Normalize()
	var total int64
	if err := d.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM objects").Scan(&total); err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListObjects:count", providererr.KindInternal)
	}

	orderBy := "size_bytes DESC"
	if p.Sort != "" {
		orderBy = p.Sort + " " + string(p.Order)
	}
	rows, err := d.q.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, key, url, size_bytes, content_type, last_modified, etag, blob_type,
			storage_class, provider, container, scan_id
		FROM objects
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, orderBy), p.Limit, p.Offset)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListObjects:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.ObjectMetadata
	for rows.Next() {
		o, err := scanObject(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *o)
	}
	return &models.ObjectPage{Items: items, Page: models.NewPageInfo(p, len(items), total)}, nil
}

func (d *DB) CountObjects(ctx context.Context) (int64, error) {
	var total int64
	err := d.q.QueryRowContext(ctx, "SELECT COUNT(*) FROM objects").Scan(&total)
	if err != nil {
		return 0, providererr.Wrap(err, "sqlite", "CountObjects", providererr.KindInternal)
	}
	return total, nil
}

func (d *DB) DeleteObjectsByContainer(ctx context.Context, container string) (int64, error) {
	res, err := d.q.ExecContext(ctx, "DELETE FROM objects WHERE container = ?", container)
	if err != nil {
		return 0, providererr.Wrap(err, "sqlite", "DeleteObjectsByContainer", providererr.KindInternal)
	}
	return res.RowsAffected()
}

func scanObject(scanner interface {
	Scan(dest ...any) error
}) (*models.ObjectMetadata, error) {
	var o models.ObjectMetadata
	var lastModified string
	err := scanner.Scan(
		&o.ID, &o.Key, &o.URL, &o.SizeBytes, &o.ContentType,
		&lastModified, &o.ETag, &o.BlobType, &o.StorageClass,
		&o.Provider, &o.Container, &o.ScanID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, providererr.NotFound("sqlite", "GetObject", "")
		}
		return nil, providererr.Wrap(err, "sqlite", "ScanObject", providererr.KindMalformedResponse)
	}
	o.LastModified, _ = time.Parse(time.RFC3339, lastModified)
	return &o, nil
}