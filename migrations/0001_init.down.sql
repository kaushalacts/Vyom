-- Down migration for v1: drop all tables in reverse dependency order
DROP TRIGGER IF EXISTS trg_providers_updated;
DROP TRIGGER IF EXISTS trg_policies_updated;
DROP TRIGGER IF EXISTS trg_objects_updated;
DROP TRIGGER IF EXISTS trg_assets_updated;

DROP INDEX IF EXISTS idx_manifest_asset;
DROP INDEX IF EXISTS idx_manifest_job_id;
DROP INDEX IF EXISTS idx_impacts_potential_savings;
DROP INDEX IF EXISTS idx_impacts_current_bytes;
DROP INDEX IF EXISTS idx_impacts_priority_score;
DROP INDEX IF EXISTS idx_impacts_asset_id;
DROP INDEX IF EXISTS idx_impacts_scan_id;
DROP INDEX IF EXISTS idx_objects_provider_container;
DROP INDEX IF EXISTS idx_objects_key;
DROP INDEX IF EXISTS idx_objects_scan_id;
DROP INDEX IF EXISTS idx_traffic_format;
DROP INDEX IF EXISTS idx_traffic_url;
DROP INDEX IF EXISTS idx_traffic_scan_id;
DROP INDEX IF EXISTS idx_assets_url;
DROP INDEX IF EXISTS idx_assets_format;
DROP INDEX IF EXISTS idx_assets_requests;
DROP INDEX IF EXISTS idx_assets_bytes_delivered;
DROP INDEX IF EXISTS idx_assets_storage_key;
DROP INDEX IF EXISTS idx_assets_scan_id;

DROP TABLE IF EXISTS manifest_entries;
DROP TABLE IF EXISTS optimization_jobs;
DROP TABLE IF EXISTS impacts;
DROP TABLE IF EXISTS objects;
DROP TABLE IF EXISTS traffic_records;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS scans;
DROP TABLE IF EXISTS policies;
DROP TABLE IF EXISTS providers;
DROP TABLE IF EXISTS schema_migrations;