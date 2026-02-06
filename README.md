# Site Selection IQ Scoring Service

**Workforce AI — Backend Engineering Case Study**

A production-grade Go backend service that ingests client CSV data, runs an asynchronous weighted scoring pipeline, and exposes ranked site recommendations through a RESTful API. Designed as a foundational service pattern for the Workforce AI platform.

## Architecture

```
                         +-----------+
                         |  Gin API  |
                         | (JWT Auth)|
                         +-----+-----+
                               |
              +----------------+----------------+
              |                |                |
        POST /uploads   POST /runs      GET /recommendations
              |                |                |
     +--------v--------+  +---v----+   +-------v--------+
     | CSV Ingest       |  |Pipeline|   | Paginated Query|
     | - SHA-256 dedup  |  | (async)|   | - min_score    |
     | - Schema resolve |  |        |   | - explanation  |
     | - Type coercion  |  +---+----+   +----------------+
     +--------+---------+      |
              |           +----v-----+
              |           | Scoring  |
              |           | Engine   |
              |           +----+-----+
              |                |
         +----v----------------v----+
         |       PostgreSQL 16      |
         | JSONB  |  RLS  |  UUIDs |
         +--------------------------+
```

### Key Design Decisions

**Schema-driven, not hardcoded.** CSV column layouts vary by tenant. Rather than hard-coding column expectations, the system uses a two-layer schema configuration (global defaults + tenant overrides) stored in the database. Adding a new customer with different columns requires a database row, not a code change.

**Asynchronous scoring with full traceability.** The scoring pipeline runs in a background goroutine after immediately returning a run ID (202 Accepted). Every run captures an immutable schema snapshot, tracks instance/transaction IDs, and records per-site factor breakdowns for auditability.

**Idempotency at every mutation.** Both upload and scoring endpoints accept idempotency keys via atomic `INSERT ... ON CONFLICT` operations. Duplicate file uploads are also caught via SHA-256 content hashing with a per-tenant unique constraint.

**Multi-tenant isolation.** Every query is scoped by `tenant_id`. JWT claims carry tenant context and role (admin/analyst/viewer) for RBAC enforcement at the middleware layer.

## Tech Stack

Go 1.23, Gin web framework, PostgreSQL 16 with pgx/v5 connection pooling, JWT (HS256) authentication, Docker multi-stage build.

Zero external services required — the scoring engine runs in-process using a configurable weighted algorithm with normalization and ranking.

## Prerequisites

- Docker and Docker Compose
- (Optional) Go 1.23+ for local development without Docker
- (Optional) `make` for convenience targets

## Quick Start

**1. Configure environment variables**

```bash
cp .env.example .env
# Edit .env — set DB_PASSWORD, JWT_SECRET, and any other values
```

**2. Start the stack**

```bash
make docker-up
# or: docker compose up --build -d
```

This starts PostgreSQL and the API server. Migrations run automatically on startup, seeding two demo tenants (Acme Logistics, Globex Distribution) and a global schema configuration.

**3. Verify it's running**

```bash
curl http://localhost:8080/health
# {"status":"healthy","service":"site-selection-iq"}
```

**4. Open the demo console**

Navigate to [http://localhost:8080](http://localhost:8080) for the interactive demo UI, or [http://localhost:8080/docs](http://localhost:8080/docs) for the Swagger API explorer.

## API Overview

All `/api/v1/*` endpoints require a `Bearer` JWT with tenant context.

| Endpoint | Method | Role | Description |
|---|---|---|---|
| `/api/v1/uploads` | POST | admin, analyst | Upload CSV with schema validation |
| `/api/v1/uploads/:upload_id/runs` | POST | admin, analyst | Trigger async scoring pipeline |
| `/api/v1/runs/:run_id` | GET | all authed | Poll run status |
| `/api/v1/runs/:run_id/recommendations` | GET | all authed | Paginated ranked results |
| `/api/v1/runs/:run_id/recommendations/:site_id/explain` | GET | all authed | Detailed factor breakdown |
| `/dev/token` | POST | none | Generate test JWT (dev only) |
| `/health` | GET | none | Health check |

The full OpenAPI 3.0 specification is served at `/openapi.yaml`.

### Example: End-to-End Flow

```bash
# 1. Get a dev token
TOKEN=$(curl -s -X POST http://localhost:8080/dev/token \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"550e8400-e29b-41d4-a716-446655440000","user_id":"00000000-0000-0000-0000-000000000001","role":"admin"}' \
  | jq -r '.token')

# 2. Upload a CSV
UPLOAD=$(curl -s -X POST http://localhost:8080/api/v1/uploads \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@testdata/logistics_site_data.csv" \
  -F "idempotency_key=$(uuidgen)")
echo $UPLOAD | jq .
UPLOAD_ID=$(echo $UPLOAD | jq -r '.data.upload_id')

# 3. Trigger scoring
RUN=$(curl -s -X POST "http://localhost:8080/api/v1/uploads/$UPLOAD_ID/runs" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"idempotency_key\":\"$(uuidgen)\"}")
RUN_ID=$(echo $RUN | jq -r '.data.run_id')

# 4. Poll until complete
curl -s "http://localhost:8080/api/v1/runs/$RUN_ID" \
  -H "Authorization: Bearer $TOKEN" | jq '.data.status'

# 5. Get recommendations
curl -s "http://localhost:8080/api/v1/runs/$RUN_ID/recommendations?page=1&page_size=10" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

## Scoring Pipeline

The scoring engine uses a weighted normalization algorithm:

1. For each numeric field in the resolved schema, extract the site's value
2. Normalize to [0, 1] using configured min/max bounds and direction (maximize or minimize)
3. Multiply by the field's weight to get a weighted contribution
4. Sum contributions, divide by max possible score, scale to 0-100

Every recommendation includes a structured explanation with per-factor breakdowns (value, weight, contribution, direction, and human-readable reason) plus a summary highlighting the top contributing factors.

The pipeline runs with configurable retry logic (exponential backoff + jitter) and creates an immutable schema config snapshot at the start of each run for auditability.

## Project Structure

```
cmd/server/             Entry point
internal/
  api/
    handlers/           Upload, Run, Recommendation handlers
    middleware/          Auth, RBAC, CORS, Logging, Correlation ID
    response/           Standard envelope wrapper
  config/               Environment-based configuration
  db/                   Connection pool, embedded migrations
  models/               Domain types (Upload, SiteRecord, ScoringRun, etc.)
  repository/           Data access layer (pgx)
  schema/               Schema resolution and CSV validation
  scoring/              Pipeline orchestration and scoring engine
  ingest/               CSV parsing
pkg/auth/               JWT generation and validation
static/                 Demo console and Swagger UI
testdata/               Sample CSVs for both demo tenants
```

## Testing

```bash
make test               # Run all tests with race detector
make test-coverage      # Generate HTML coverage report
```

Tests cover JWT token lifecycle, schema resolution and merging, CSV header/row validation, and the scoring algorithm (normalization, weighting, ranking).

## Development

```bash
make build              # Compile binary to bin/
make run                # Build and run locally (requires Postgres)
make fmt                # Format Go source
make vet                # Run static analysis
make deps               # Tidy and download modules
make logs               # Tail API container logs
make docker-clean       # Stop and remove volumes
```

## Configuration

All configuration is via environment variables. See `.env.example` for the complete list with descriptions. Key settings:

| Variable | Purpose |
|---|---|
| `DB_PASSWORD` | PostgreSQL password |
| `JWT_SECRET` | HMAC signing key for JWTs |
| `UPLOAD_MAX_SIZE_MB` | Max CSV file size (default 100) |
| `SCORING_MAX_RETRIES` | Pipeline retry attempts (default 3) |
| `SCORING_WORKER_COUNT` | Concurrent scoring workers (default 4) |

## Case Study Narrative

The detailed written responses to the case study tasks — framing and assumptions (Task 1), reliability and production readiness (Task 3), data modeling and scaling (Task 4), and communication (Task 5) — are in [`case_study_narrative.md`](case_study_narrative.md).

## What's Stubbed / Not Built

- **Virus scanning** — documented insertion point in the upload handler
- **Real ML model** — the `ScoreFunc` interface accepts any scoring implementation; the current weighted algorithm is a placeholder
- **LLM narrative** — the explanation endpoint returns structured factors; an LLM summary field is stubbed
- **Observability stack** — structured JSON logs are written to stdout in a format compatible with ELK/Datadog/CloudWatch
- **Production infra** — no Kubernetes, load balancing, or CDN
