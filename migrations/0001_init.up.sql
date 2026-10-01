-- Vyom database schema v1
-- This migration creates all tables required for Milestone 1.
-- SQLite dialect. The same DDL works on PostgreSQL with minor type changes.

-- Schema version tracking
CREATE TABLE schema_migrations (
    version     INTEGER PRIMARY KEY,
    checksum    TEXT NOT NULL,
    applied_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Providers (CDN and storage)
CREATE TABLE providers (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL CHECK (kind IN ('cdn','storage')),
    type            TEXT NOT NULL,
    name            TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    options_json    TEXT NOT NULL DEFAULT '{}',
    credentials_json TEXT NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'unconfigured',
    last_checked    TEXT,
    last_error      TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Optimization policies
CREATE TABLE policies (
    name            TEXT PRIMARY KEY,
    version         TEXT NOT NULL,
    description     TEXT,
    images_json     TEXT NOT NULL,
    video_json      TEXT NOT NULL DEFAULT '{}',
    pricing_json    TEXT NOT NULL DEFAULT '{}',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Scan jobs
CREATE TABLE scans (
    id                  TEXT PRIMARY KEY,
    status              TEXT NOT NULL CHECK (status IN ('pending','running','completed','partial','failed','cancelled')),
    current_stage       TEXT,
    progress_pct        REAL NOT NULL DEFAULT 0,
    window_start        TEXT NOT NULL,
    window_end          TEXT NOT NULL,
    cdn_provider        TEXT,
    storage_provider    TEXT,
    policy_name         TEXT,
    counts_json         TEXT NOT NULL DEFAULT '{}',
    warnings_json       TEXT NOT NULL DEFAULT '[]',
    error               TEXT,
    source              TEXT,
    started_at          TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at         TEXT
);

-- Assets (correlated CDN+storage objects)
CREATE TABLE assets (
    id                  TEXT PRIMARY KEY,
    url                 TEXT NOT NULL,
    storage_key         TEXT NOT NULL,
    storage_provider    TEXT,
    cdn_provider        TEXT,
    content_type        TEXT,
    format              TEXT,
    size_bytes          INTEGER NOT NULL DEFAULT 0,
    width               INTEGER NOT NULL DEFAULT 0,
    height              INTEGER NOT NULL DEFAULT 0,
    requests            INTEGER NOT NULL DEFAULT 0,
    bytes_delivered     INTEGER NOT NULL DEFAULT 0,
    first_seen          TEXT NOT NULL,
    last_seen           TEXT NOT NULL,
    last_modified       TEXT,
    etag                TEXT,
    content_hash        TEXT,
    scan_id             TEXT NOT NULL,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (scan_id) REFERENCES scans(id)
);

-- CDN traffic records
CREATE TABLE traffic_records (
    id                  TEXT PRIMARY KEY,
    url                 TEXT NOT NULL,
    requests            INTEGER NOT NULL DEFAULT 0,
    bytes_delivered     INTEGER NOT NULL DEFAULT 0,
    start_time          TEXT NOT NULL,
    end_time            TEXT NOT NULL,
    content_type        TEXT,
    format              TEXT,
    status_code         INTEGER,
    country             TEXT,
    provider            TEXT,
    scan_id             TEXT NOT NULL,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (scan_id) REFERENCES scans(id)
);

-- Storage object metadata
CREATE TABLE objects (
    id                  TEXT PRIMARY KEY,
    key                 TEXT NOT NULL,
    url                 TEXT,
    size_bytes          INTEGER NOT NULL DEFAULT 0,
    content_type        TEXT,
    last_modified       TEXT NOT NULL,
    etag                TEXT,
    blob_type           TEXT,
    storage_class       TEXT,
    provider            TEXT NOT NULL,
    container           TEXT NOT NULL,
    scan_id             TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (scan_id) REFERENCES scans(id)
);

-- Asset impact scores
CREATE TABLE impacts (
    id                      TEXT PRIMARY KEY,
    asset_id                TEXT NOT NULL,
    url                     TEXT NOT NULL,
    storage_key             TEXT,
    requests                INTEGER NOT NULL DEFAULT 0,
    current_bytes           INTEGER NOT NULL DEFAULT 0,
    projected_bytes         INTEGER NOT NULL DEFAULT 0,
    potential_savings       INTEGER NOT NULL DEFAULT 0,
    savings_percent         REAL NOT NULL DEFAULT 0,
    priority_score          REAL NOT NULL DEFAULT 0,
    delivered_share         REAL NOT NULL DEFAULT 0,
    savings_share           REAL NOT NULL DEFAULT 0,
    request_share           REAL NOT NULL DEFAULT 0,
    estimated_optimized_size INTEGER NOT NULL DEFAULT 0,
    class                   TEXT NOT NULL DEFAULT 'low',
    reasons_json            TEXT NOT NULL DEFAULT '[]',
    scan_id                 TEXT NOT NULL,
    created_at              TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (asset_id) REFERENCES assets(id),
    FOREIGN KEY (scan_id) REFERENCES scans(id)
);

-- Optimization jobs
CREATE TABLE optimization_jobs (
    id                  TEXT PRIMARY KEY,
    dry_run             INTEGER NOT NULL DEFAULT 1,
    status              TEXT NOT NULL CHECK (status IN ('pending','running','completed','partial','failed','cancelled','rolled_back')),
    policy_name         TEXT,
    scan_id             TEXT,
    counts_json         TEXT NOT NULL DEFAULT '{}',
    started_at          TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at         TEXT,
    error               TEXT,
    FOREIGN KEY (scan_id) REFERENCES scans(id)
);

-- Optimization manifest entries
CREATE TABLE manifest_entries (
    id                      TEXT PRIMARY KEY,
    job_id                  TEXT NOT NULL,
    asset                   TEXT NOT NULL,
    original_size           INTEGER NOT NULL,
    optimized_size          INTEGER NOT NULL,
    original_format         TEXT NOT NULL,
    optimized_format        TEXT NOT NULL,
    ssim                    REAL NOT NULL DEFAULT 0,
    psnr                    REAL NOT NULL DEFAULT 0,
    original_key            TEXT,
    optimized_key           TEXT,
    original_hash           TEXT,
    optimized_hash          TEXT,
    requests                INTEGER NOT NULL DEFAULT 0,
    bytes_delivered         INTEGER NOT NULL DEFAULT 0,
    estimated_savings_bytes INTEGER NOT NULL DEFAULT 0,
    timestamp               TEXT NOT NULL DEFAULT (datetime('now')),
    operation               TEXT NOT NULL DEFAULT 'generate-copy',
    status                  TEXT NOT NULL DEFAULT 'success',
    dry_run                 INTEGER NOT NULL DEFAULT 1,
    reason                  TEXT,
    FOREIGN KEY (job_id) REFERENCES optimization_jobs(id)
);

-- Indexes for query performance
CREATE INDEX idx_assets_scan_id ON assets(scan_id);
CREATE INDEX idx_assets_storage_key ON assets(storage_key);
CREATE INDEX idx_assets_bytes_delivered ON assets(bytes_delivered DESC);
CREATE INDEX idx_assets_requests ON assets(requests DESC);
CREATE INDEX idx_assets_format ON assets(format);
CREATE INDEX idx_assets_url ON assets(url);

CREATE INDEX idx_traffic_scan_id ON traffic_records(scan_id);
CREATE INDEX idx_traffic_url ON traffic_records(url);
CREATE INDEX idx_traffic_format ON traffic_records(format);

CREATE INDEX idx_objects_scan_id ON objects(scan_id);
CREATE INDEX idx_objects_key ON objects(key);
CREATE INDEX idx_objects_provider_container ON objects(provider, container);

CREATE INDEX idx_impacts_scan_id ON impacts(scan_id);
CREATE INDEX idx_impacts_asset_id ON impacts(asset_id);
CREATE INDEX idx_impacts_priority_score ON impacts(priority_score DESC);
CREATE INDEX idx_impacts_current_bytes ON impacts(current_bytes DESC);
CREATE INDEX idx_impacts_potential_savings ON impacts(potential_savings DESC);

CREATE INDEX idx_manifest_job_id ON manifest_entries(job_id);
CREATE INDEX idx_manifest_asset ON manifest_entries(asset);

-- Trigger to maintain updated_at
CREATE TRIGGER trg_assets_updated
AFTER UPDATE ON assets
BEGIN
    UPDATE assets SET updated_at = datetime('now') WHERE id = NEW.id;
END;

CREATE TRIGGER trg_objects_updated
AFTER UPDATE ON objects
BEGIN
    UPDATE objects SET updated_at = datetime('now') WHERE id = NEW.id;
END;

CREATE TRIGGER trg_policies_updated
AFTER UPDATE ON policies
BEGIN
    UPDATE policies SET updated_at = datetime('now') WHERE name = NEW.name;
END;

CREATE TRIGGER trg_providers_updated
AFTER UPDATE ON providers
BEGIN
    UPDATE providers SET updated_at = datetime('now') WHERE id = NEW.id;
END;