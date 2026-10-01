// Package repository defines Vyom's persistence contract.
//
// The repository layer is the only code that knows SQL. All domain logic
// operates on interfaces, so swapping SQLite for PostgreSQL in a later
// milestone only means providing a new implementation of these interfaces.
package repository

import (
	"context"
	"time"

	"github.com/kaushalacts/vyom/pkg/models"
)

// AssetRepo stores and queries assets.
type AssetRepo interface {
	UpsertAsset(ctx context.Context, a *models.Asset) error
	GetAsset(ctx context.Context, id string) (*models.Asset, error)
	ListAssets(ctx context.Context, f models.AssetFilter, p models.PageRequest) (*models.AssetPage, error)
	CountAssets(ctx context.Context, f models.AssetFilter) (int64, error)
	DeleteAsset(ctx context.Context, id string) error
	DeleteAssetsByScan(ctx context.Context, scanID string) (int64, error)
}

// TrafficRepo stores and queries CDN traffic records.
type TrafficRepo interface {
	UpsertTrafficBatch(ctx context.Context, records []models.TrafficRecord) error
	ListTraffic(ctx context.Context, p models.PageRequest) (*models.TrafficPage, error)
	CountTraffic(ctx context.Context) (int64, error)
	DeleteTrafficByWindow(ctx context.Context, start, end time.Time) (int64, error)
}

// ObjectRepo stores and queries storage object metadata.
type ObjectRepo interface {
	UpsertObjectBatch(ctx context.Context, objs []models.ObjectMetadata) error
	GetObject(ctx context.Context, key string) (*models.ObjectMetadata, error)
	ListObjects(ctx context.Context, p models.PageRequest) (*models.ObjectPage, error)
	CountObjects(ctx context.Context) (int64, error)
	DeleteObjectsByContainer(ctx context.Context, container string) (int64, error)
}

// ImpactRepo stores and queries asset impact scores.
type ImpactRepo interface {
	UpsertImpactBatch(ctx context.Context, impacts []models.AssetImpact) error
	ListImpacts(ctx context.Context, p models.PageRequest) (*models.ImpactPage, error)
	TopImpacts(ctx context.Context, limit int) ([]models.AssetImpact, error)
	ImpactsByAsset(ctx context.Context, assetID string) ([]models.AssetImpact, error)
	DeleteImpactsByScan(ctx context.Context, scanID string) (int64, error)
}

// ScanRepo stores and queries scan jobs.
type ScanRepo interface {
	CreateScan(ctx context.Context, job *models.ScanJob) error
	UpdateScan(ctx context.Context, job *models.ScanJob) error
	GetScan(ctx context.Context, id string) (*models.ScanJob, error)
	ListScans(ctx context.Context, p models.PageRequest) (*models.ScanPage, error)
	LatestScan(ctx context.Context) (*models.ScanJob, error)
}

// OptimizationRepo stores and queries optimization jobs and manifests.
type OptimizationRepo interface {
	CreateOptimizationJob(ctx context.Context, job *models.OptimizationJob) error
	UpdateOptimizationJob(ctx context.Context, job *models.OptimizationJob) error
	GetOptimizationJob(ctx context.Context, id string) (*models.OptimizationJob, error)
	ListOptimizationJobs(ctx context.Context, p models.PageRequest) (*models.OptimizationJobPage, error)
	AppendManifest(ctx context.Context, entries []models.ManifestEntry) error
	GetManifest(ctx context.Context, jobID string) ([]models.ManifestEntry, error)
}

// PolicyRepo stores and queries optimization policies.
type PolicyRepo interface {
	UpsertPolicy(ctx context.Context, p *models.OptimizationPolicy) error
	GetPolicy(ctx context.Context, name string) (*models.OptimizationPolicy, error)
	ListPolicies(ctx context.Context) ([]models.OptimizationPolicy, error)
	DeletePolicy(ctx context.Context, name string) error
}

// ProviderRepo stores and queries provider configurations.
type ProviderRepo interface {
	UpsertProvider(ctx context.Context, p *models.ProviderConfig) error
	GetProvider(ctx context.Context, id string) (*models.ProviderConfig, error)
	ListProviders(ctx context.Context) ([]models.ProviderConfig, error)
	DeleteProvider(ctx context.Context, id string) error
}

// DB is the aggregate interface a scan or API handler uses.
//
// It groups all repositories so that a handler can receive one value instead of
// seven, and so that a transaction can span multiple repositories.
type DB interface {
	AssetRepo
	TrafficRepo
	ObjectRepo
	ImpactRepo
	ScanRepo
	OptimizationRepo
	PolicyRepo
	ProviderRepo

	// Tx runs fn inside a transaction. The transaction is committed if fn
	// returns nil, rolled back otherwise. Nested calls are not supported in
	// SQLite; callers must not call Tx inside a Tx.
	Tx(ctx context.Context, fn func(DB) error) error

	// Close releases the underlying connection pool.
	Close() error

	// Migrate applies pending migrations. It is idempotent and safe to call
	// at startup.
	Migrate(ctx context.Context) error
}