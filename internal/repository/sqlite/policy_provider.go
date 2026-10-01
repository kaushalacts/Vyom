package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func (d *DB) UpsertPolicy(ctx context.Context, p *models.OptimizationPolicy) error {
	imagesJSON, _ := json.Marshal(p.Images)
	videoJSON, _ := json.Marshal(p.Video)
	pricingJSON, _ := json.Marshal(p.Pricing)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.q.ExecContext(ctx, `
		INSERT INTO policies (name, version, description, images_json, video_json, pricing_json,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			version = excluded.version,
			description = excluded.description,
			images_json = excluded.images_json,
			video_json = excluded.video_json,
			pricing_json = excluded.pricing_json,
			updated_at = excluded.updated_at
	`, p.Name, p.Version, p.Description, string(imagesJSON), string(videoJSON),
		string(pricingJSON), now, now)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "UpsertPolicy", providererr.KindInternal)
	}
	return nil
}

func (d *DB) GetPolicy(ctx context.Context, name string) (*models.OptimizationPolicy, error) {
	row := d.q.QueryRowContext(ctx, `
		SELECT name, version, description, images_json, video_json, pricing_json
		FROM policies WHERE name = ?
	`, name)
	return scanPolicy(row)
}

func (d *DB) ListPolicies(ctx context.Context) ([]models.OptimizationPolicy, error) {
	rows, err := d.q.QueryContext(ctx, `
		SELECT name, version, description, images_json, video_json, pricing_json
		FROM policies ORDER BY name
	`)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListPolicies:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.OptimizationPolicy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return items, nil
}

func (d *DB) DeletePolicy(ctx context.Context, name string) error {
	_, err := d.q.ExecContext(ctx, "DELETE FROM policies WHERE name = ?", name)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "DeletePolicy", providererr.KindInternal)
	}
	return nil
}

func scanPolicy(scanner interface {
	Scan(dest ...any) error
}) (*models.OptimizationPolicy, error) {
	var p models.OptimizationPolicy
	var imagesJSON, videoJSON, pricingJSON string
	err := scanner.Scan(
		&p.Name, &p.Version, &p.Description, &imagesJSON, &videoJSON, &pricingJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, providererr.NotFound("sqlite", "GetPolicy", "")
		}
		return nil, providererr.Wrap(err, "sqlite", "ScanPolicy", providererr.KindMalformedResponse)
	}
	_ = json.Unmarshal([]byte(imagesJSON), &p.Images)
	_ = json.Unmarshal([]byte(videoJSON), &p.Video)
	_ = json.Unmarshal([]byte(pricingJSON), &p.Pricing)
	return &p, nil
}

func (d *DB) UpsertProvider(ctx context.Context, p *models.ProviderConfig) error {
	if p.ID == "" {
		p.ID = models.NewID(models.PrefixProvider)
	}
	optionsJSON, _ := json.Marshal(p.Options)
	credentialsJSON, _ := json.Marshal(p.Credentials)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.q.ExecContext(ctx, `
		INSERT INTO providers (id, kind, type, name, enabled, options_json, credentials_json,
			status, last_checked, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind = excluded.kind, type = excluded.type, name = excluded.name,
			enabled = excluded.enabled, options_json = excluded.options_json,
			credentials_json = excluded.credentials_json, status = excluded.status,
			last_checked = excluded.last_checked, last_error = excluded.last_error,
			updated_at = excluded.updated_at
	`, p.ID, p.Kind, p.Type, p.Name, p.Enabled, string(optionsJSON),
		string(credentialsJSON), p.Status, nullTime(p.LastChecked),
		p.LastError, now, now)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "UpsertProvider", providererr.KindInternal)
	}
	return nil
}

func (d *DB) GetProvider(ctx context.Context, id string) (*models.ProviderConfig, error) {
	row := d.q.QueryRowContext(ctx, `
		SELECT id, kind, type, name, enabled, options_json, credentials_json,
			status, last_checked, last_error, created_at, updated_at
		FROM providers WHERE id = ?
	`, id)
	return scanProvider(row)
}

func (d *DB) ListProviders(ctx context.Context) ([]models.ProviderConfig, error) {
	rows, err := d.q.QueryContext(ctx, `
		SELECT id, kind, type, name, enabled, options_json, credentials_json,
			status, last_checked, last_error, created_at, updated_at
		FROM providers ORDER BY kind, type, name
	`)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "ListProviders:query", providererr.KindInternal)
	}
	defer rows.Close()

	var items []models.ProviderConfig
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return items, nil
}

func (d *DB) DeleteProvider(ctx context.Context, id string) error {
	_, err := d.q.ExecContext(ctx, "DELETE FROM providers WHERE id = ?", id)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "DeleteProvider", providererr.KindInternal)
	}
	return nil
}

func scanProvider(scanner interface {
	Scan(dest ...any) error
}) (*models.ProviderConfig, error) {
	var p models.ProviderConfig
	var optionsJSON, credentialsJSON, createdAt, updatedAt, lastCheckedStr string
	var lastChecked sql.NullString
	err := scanner.Scan(
		&p.ID, &p.Kind, &p.Type, &p.Name, &p.Enabled,
		&optionsJSON, &credentialsJSON, &p.Status,
		&lastCheckedStr, &p.LastError, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, providererr.NotFound("sqlite", "GetProvider", "")
		}
		return nil, providererr.Wrap(err, "sqlite", "ScanProvider", providererr.KindMalformedResponse)
	}
	_ = json.Unmarshal([]byte(optionsJSON), &p.Options)
	_ = json.Unmarshal([]byte(credentialsJSON), &p.Credentials)
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if lastCheckedStr != "" {
		p.LastChecked, _ = time.Parse(time.RFC3339, lastCheckedStr)
	}
	return &p, nil
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}