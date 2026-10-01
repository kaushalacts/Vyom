# Vyom

[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()

**Vyom** (व्योम — sky, space, vast expanse) is an open-source CDN media observability and optimization platform.

> **Tagline:** "Find the media assets consuming your CDN bandwidth."

## The Problem

A production incident revealed CDN bandwidth unexpectedly doubled from ~15 TB/month to ~30 TB/month. Investigation showed:

- Images stored in Azure Blob Storage served through a CDN
- Individual images: 300 KB–1 MB (some 700 KB–1 MB)
- Some assets requested hundreds of thousands to millions of times
- Aggregate bandwidth became massive, though no single file seemed problematic

A Go prototype identified images above a configurable size threshold and optimized them, achieving substantial size reductions while preserving visual quality.

Vyom generalizes this into a **vendor-neutral media observability and optimization platform**.

## Core Value Proposition

> **Tell me exactly which media assets are responsible for my CDN bandwidth, why they are expensive, and how much bandwidth I could save by optimizing them.**

Vyom answers five questions:

1. **How much media traffic is my CDN serving?**
2. **Which assets consume the most bandwidth?**
3. **Why did my CDN traffic increase?**
4. **How much could I potentially save by optimizing these assets?**
5. **Can Vyom safely optimize them without degrading quality?**

## Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌────────────────────┐
│   CDN       │     │   Vyom Core      │     │  Object Storage    │
│  (Akamai)   │────▶│  (Correlation,   │────▶│  (Azure Blob,      │
│  CloudFront │     │   Impact, Policy)│     │   S3, GCS, R2)     │
│  Cloudflare)│     │                  │     │                    │
└─────────────┘     └──────────────────┘     └────────────────────┘
                           │                        │
                           ▼                        ▼
                    ┌──────────────────┐     ┌────────────────────┐
                    │   Reports &      │     │   Optimizer        │
                    │   Dashboard      │     │   (libvips)        │
                    └──────────────────┘     └────────────────────┘
```

### Pipeline

```
START
 ↓
Validate configuration
 ↓
Fetch CDN traffic
 ↓
Fetch storage inventory
 ↓
Normalize data
 ↓
Correlate CDN assets → storage objects
 ↓
Detect media (JPEG, PNG, WebP)
 ↓
Calculate impact (delivered bytes, requests, optimization potential)
 ↓
Persist results
 ↓
Generate report
 ↓
END
```

### Asset Impact Formula

Vyom **does not** use a simple size threshold. Instead, it calculates **Asset Impact** based on:

- `current_delivered_bytes` — bytes served in the scan window
- `request_count` — how often the asset was requested
- `projected_optimized_size` — estimated size after policy-based optimization
- `potential_savings = current_delivered_bytes - projected_delivered_bytes`
- `priority_score` — weighted combination (default: 50% savings, 30% delivered bytes, 20% requests)

A 900 KB image requested 10 times → low priority  
A 900 KB image requested 5M times → **critical priority**

## Features

### Milestone 1 (Current)
- [x] Go project structure with clean architecture
- [x] Domain models (Asset, TrafficRecord, ObjectMetadata, AssetImpact, OptimizationPolicy, etc.)
- [x] Provider interfaces (CDN, Storage, Optimizer)
- [x] SQLite persistence with migrations
- [x] Configuration (YAML + env vars + CLI overrides)
- [x] Structured logging with secret redaction
- [x] Error taxonomy (`providererr`)
- [x] Policy engine (validation, evaluation, impact scoring)
- [x] CLI (`vyom version`, `init`, `doctor`, `migrate`, `config`, `provider`, `policy`, `scan`, `report`, `optimize`, `serve`)
- [x] REST API skeleton (`/api/v1/health`, `/version`, assets, impacts, scans, optimizations, policies, providers)
- [x] Docker support

### Planned Milestones
- **M2:** Azure Blob connector
- **M3:** Akamai connector
- **M4:** Correlation engine
- **M5:** Impact analysis & reports
- **M6:** Image optimization (libvips)
- **M7:** Quality validation (SSIM, PSNR)
- **M8:** REST API completion
- **M9:** Dashboard (React + TypeScript)
- **M10:** Docker Compose, Prometheus metrics
- **M11:** GitHub Action for CI/CD
- **M12:** Helm chart / Kubernetes

## Quick Start

```bash
# Build
go build -o vyom ./cmd/vyom

# Initialize project (creates vyom.yaml and vyom.db)
./vyom init

# Check configuration
./vyom doctor

# Add providers (when implemented in M2/M3)
# ./vyom provider add cdn akamai --name prod --host <host> --client-token-env VYOM_AKAMAI_CLIENT_TOKEN ...
# ./vyom provider add storage azure_blob --name prod --account <account> --container <container> ...

# Run a scan (when providers are implemented)
# ./vyom scan --from 2026-09-01 --to 2026-09-30

# View report
# ./vyom report --format json

# Simulate optimization
# ./vyom optimize --dry-run
```

## Configuration

Vyom uses a layered configuration system:

**Priority:** CLI flags → Environment variables → Config file (`vyom.yaml`) → Defaults

Example `vyom.yaml`:
```yaml
database:
  path: "vyom.db"

logging:
  level: "info"
  format: "json"

server:
  host: "127.0.0.1"
  port: 8080

providers:
  cdn:
    id: ""  # references provider in DB
  storage:
    id: ""

scan:
  days_back: 30
  timezone: "UTC"

policy:
  name: "default"

optimization:
  dry_run: true
  concurrency: 2
```

**Secrets are NEVER stored in the config file.** Provider credentials reference environment variable names:
```yaml
credentials:
  client_token_env: "VYOM_AKAMAI_CLIENT_TOKEN"
  client_secret_env: "VYOM_AKAMAI_CLIENT_SECRET"
  access_token_env: "VYOM_AKAMAI_ACCESS_TOKEN"
```

## CLI Reference

```
vyom --help

Available Commands:
  completion   Generate shell completion scripts
  config       Manage configuration (show, validate)
  doctor       Check provider connectivity and configuration
  init         Initialize a new Vyom project (config + database)
  migrate      Run database migrations
  optimize     Run optimization (dry-run by default)
  policy       Manage optimization policies (list, show, set, validate)
  provider     Manage provider configurations (add, list, remove, show)
  report       Generate reports from scan results
  scan         Run a scan (fetch traffic, inventory, correlate, analyze)
  serve        Start the REST API server
  version      Print version information
```

### Key Safety Principles

- **Dry-run by default:** `vyom optimize` never modifies production objects unless `--apply` is explicitly passed with confirmation
- **Never overwrites originals:** Optimized copies are written alongside originals with a new key
- **Manifest for audit/rollback:** Every optimization generates a manifest with hashes, quality metrics, and operation status
- **No secrets in logs/DB:** Credentials only live in environment variables; automatically redacted from all output

## Development

```bash
# Prerequisites: Go 1.27+, SQLite (modernc.org/sqlite is pure Go)

# Run tests
go test ./...

# Lint
golangci-lint run

# Static analysis
go vet ./...
staticcheck ./...

# Build
go build ./cmd/vyom

# Run
./vyom version
```

## Project Structure

```
vyom/
├── cmd/vyom/           # CLI entry point
├── internal/
│   ├── api/            # REST API handlers
│   ├── cdn/            # CDN provider interfaces
│   ├── config/         # Configuration loading
│   ├── logging/        # Structured logging + redaction
│   ├── optimizer/      # Optimizer interfaces
│   ├── policy/         # Policy engine + impact scoring
│   ├── repository/     # Persistence interfaces + SQLite impl
│   ├── storage/        # Storage provider interfaces
│   ├── transport/      # Shared HTTP/logger interfaces
│   └── version/        # Build metadata
├── pkg/models/         # Shared domain models (no provider deps)
├── migrations/         # SQL migrations (embedded)
├── configs/            # Example configuration
├── deployments/        # Docker, Helm (future)
└── docs/               # Documentation (future)
```

## Security

- **No hardcoded credentials** — all secrets via environment variables
- **Automatic redaction** — tokens, signatures, SAS URLs, pre-signed URLs stripped from logs
- **SSRF protection** — URL validation on all outbound requests
- **No arbitrary command execution** — image processing via libvips (future), not shell
- **Input validation** — all user input validated at API boundary
- **Read-only by default** — optimization requires explicit `--apply` + confirmation

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Roadmap

See [docs/roadmap.md](docs/roadmap.md) for detailed milestones.

---

**Vyom** — Observe → Understand → Optimize → Verify