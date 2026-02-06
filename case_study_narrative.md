# Workforce AI — Site Selection IQ Scoring Service
## Backend Engineering Case Study

**Author:** John Bear
**Date:** February 2026

---

## Task 1: Framing, Assumptions, and Data Contract

### Approach

This case study delivers a production-grade scoring pipeline, not a prototype. The system is designed as a foundational service pattern that generalizes beyond Site Selection IQ to support any model workflow on the Workforce AI platform. Every design decision — from schema configuration to observability contracts — is made with the assumption that this is the first service in a platform that will support multiple models, multiple tenants, and eventually agentic orchestration.

The key architectural insight driving this design: Workforce AI's customers are not uniform. A logistics company evaluating distribution center sites has fundamentally different data than a grocery chain planning franchise expansion or a healthcare system modeling staffing retention. The system must handle this diversity without per-customer engineering work. Columns are data. Schema is configuration. Models are swappable implementations behind stable interfaces.

### Assumptions

**What I will build:**

- Complete REST API in Go (Gin framework) with JWT authentication and tenant isolation
- Flexible CSV ingest with schema-driven validation (not hardcoded columns)
- Asynchronous scoring pipeline with full traceability (instance ID, transaction ID, correlation through every step)
- Two-layer explainability: deterministic structured explanations plus an optional LLM narrative endpoint
- Postgres-backed storage with JSONB for flexible fields and configuration snapshotting
- Idempotency key support for duplicate suppression
- Automated tests for upload endpoint and scoring logic
- Docker Compose for one-command startup (Go service + Postgres)
- Synthetic CSV dataset for end-to-end demonstration
- OpenAPI specification

**What I will stub or document:**

- Virus/malware scanning on file upload (documented insertion point in the pipeline)
- Real model service integration (placeholder weighted scoring function with documented interface)
- LLM narrative generation (documented API call pattern, may implement if time permits)
- Full ELK/observability stack (structured JSON log contract designed for platform-wide observability; see Observability section)
- HRIS/ATS connector integration (out of scope for case study, but the ingest interface is designed to support it)

**What I will not build:**

- Production deployment infrastructure (Kubernetes, load balancing, CDN)
- User management UI or registration flows
- Real ML model training or inference

### API Contract

All endpoints return consistent response envelopes. All requests require a valid JWT with tenant context. All responses include a correlation ID for traceability.

#### Standard Response Envelope

```json
{
  "status": "success" | "error",
  "data": { },
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable description",
    "details": [ ]
  },
  "meta": {
    "correlation_id": "uuid",
    "timestamp": "ISO-8601"
  }
}
```

#### POST /api/v1/uploads

Upload a CSV file for the authenticated tenant.

**Request:** multipart/form-data with file field and optional idempotency_key header.

**Response (201 Created):**
```json
{
  "status": "success",
  "data": {
    "upload_id": "uuid",
    "tenant_id": "uuid",
    "filename": "logistics_site_data.csv",
    "row_count": 150,
    "schema_version": "v1.2",
    "validation_status": "valid",
    "validation_warnings": [
      "Optional column 'warehouse_sq_footage' not in global schema; mapped via tenant config"
    ],
    "created_at": "ISO-8601"
  }
}
```

**Error cases:**
- 400: Invalid file type (not CSV), file exceeds size limit, schema validation failure (missing required columns, type mismatches)
- 409: Duplicate upload (idempotency key match)
- 413: File too large

#### POST /api/v1/uploads/{upload_id}/runs

Trigger an asynchronous scoring run against a validated upload. Returns immediately with a run ID.

**Request:**
```json
{
  "idempotency_key": "client-generated-uuid",
  "scoring_config": {
    "model_version": "latest",
    "weight_overrides": {}
  }
}
```

**Response (202 Accepted):**
```json
{
  "status": "success",
  "data": {
    "run_id": "uuid",
    "upload_id": "uuid",
    "status": "queued",
    "created_at": "ISO-8601"
  }
}
```

**Error cases:**
- 404: Upload not found or does not belong to tenant
- 409: Duplicate run (idempotency key match), returns existing run_id
- 422: Upload failed validation, cannot score

#### GET /api/v1/runs/{run_id}

Retrieve run status and metadata.

**Response (200):**
```json
{
  "status": "success",
  "data": {
    "run_id": "uuid",
    "upload_id": "uuid",
    "status": "succeeded",
    "row_count": 150,
    "scored_count": 150,
    "model_version": "site-selection-iq-v1.0",
    "schema_config_snapshot_id": "uuid",
    "instance_id": "uuid",
    "transaction_id": "uuid",
    "started_at": "ISO-8601",
    "completed_at": "ISO-8601",
    "duration_ms": 2340,
    "attempt_count": 1
  }
}
```

#### GET /api/v1/runs/{run_id}/recommendations

Retrieve ranked recommendations with pagination.

**Request query params:** `?page=1&page_size=20&min_score=50`

**Response (200):**
```json
{
  "status": "success",
  "data": {
    "run_id": "uuid",
    "recommendations": [
      {
        "rank": 1,
        "site_id": "SITE-047",
        "final_score": 87.4,
        "raw_score": 78.0,
        "explanation": {
          "factors": [
            { "name": "unemployment_rate", "value": 6.2, "weight": 1.3, "contribution": +4.8, "direction": "positive", "reason": "Above-average unemployment suggests available labor pool" },
            { "name": "local_competitors", "value": 2, "weight": -0.8, "contribution": +3.2, "direction": "positive", "reason": "Low competitor density reduces hiring competition" },
            { "name": "labor_cost_index", "value": 72, "weight": -0.5, "contribution": -1.6, "direction": "negative", "reason": "Above-median labor costs in this market" }
          ],
          "summary": "Score adjusted +9.4 from raw. Primary drivers: available labor pool and low competition."
        }
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total_results": 150,
      "total_pages": 8
    }
  }
}
```

#### GET /api/v1/runs/{run_id}/recommendations/{site_id}/explain

Retrieve detailed explanation for a specific site, optionally including LLM-generated narrative.

**Request query params:** `?include_narrative=true`

**Response (200):**
```json
{
  "status": "success",
  "data": {
    "site_id": "SITE-047",
    "run_id": "uuid",
    "final_score": 87.4,
    "raw_score": 78.0,
    "explanation": {
      "factors": [ ],
      "weights_applied": {
        "source": "tenant_override",
        "schema_config_snapshot_id": "uuid",
        "weight_set": {
          "unemployment_rate": 1.3,
          "local_competitors": -0.8,
          "labor_cost_index": -0.5,
          "avg_commute_time": -0.3,
          "working_age_pop": 1.0,
          "public_transport_access": 0.6
        }
      },
      "model_version": "site-selection-iq-v1.0",
      "scored_at": "ISO-8601"
    },
    "narrative": "Site 47 ranked highest among all evaluated locations, driven primarily by its large working-age population and low competitive density. The local unemployment rate of 6.2% suggests a readily available labor pool without signaling broader economic distress. While labor costs are slightly above median for the region, this is offset by strong public transit access, which historically correlates with lower turnover in distribution center roles. Compared to the next-ranked site (Site 12), Site 47 offers a materially better balance of workforce availability and cost efficiency.",
    "narrative_metadata": {
      "generated_by": "llm",
      "model": "claude-sonnet",
      "generated_at": "ISO-8601",
      "source": "deterministic_explanation",
      "disclaimer": "Narrative generated from structured scoring data. See explanation.factors for auditable source."
    }
  }
}
```

### Core Entities and Relationships

```
Tenant (1) ──── has many ──── SchemaConfig (per-tenant overrides)
  │                                │
  │                                ▼
  ├── has many ── Upload (1) ── validated against ── ResolvedSchema
  │                  │              (global defaults + tenant overrides)
  │                  │
  │                  ├── has many ── SiteRecord (parsed CSV rows, JSONB dynamic fields)
  │                  │
  │                  └── has many ── ScoringRun
  │                                    │
  │                                    ├── snapshot of ── SchemaConfigSnapshot
  │                                    ├── references ── ModelVersion
  │                                    └── has many ── Recommendation
  │                                                      │
  │                                                      └── has one ── Explanation (JSONB)
  │
  └── GlobalSchemaConfig (system-wide defaults, base field definitions, default weights)
```

**Key design decisions:**

- **SchemaConfig is hierarchical:** Global defaults define the baseline fields, types, valid ranges, and default weights. Tenant-level configs extend or override — adding custom columns, adjusting weights, modifying validation rules. Model-level schema configuration is flagged as a future extension point but not built yet.
- **SiteRecord uses JSONB for dynamic fields:** Fixed columns for common identifiers (site_id, tenant_id, upload_id). JSONB column for all data fields. This supports the "columns are configuration" design — a new customer onboards with their own dataset shape without an engineering change.
- **ScoringRun snapshots configuration:** When a run executes, the resolved schema config and weights are captured as they were at that moment. If weights change later, historical runs remain fully explainable. This is the auditability backbone.
- **Explanation is structured data, not text:** The deterministic explanation is a JSONB object with a defined contract. The LLM narrative layer is optional and stateless — it consumes the structured explanation, it does not replace it. System works fully without the LLM.

### Authentication and Authorization

**JWT-based authentication** for the WBA front-end. The user authenticates (username/password or SSO), receives a signed JWT containing tenant_id, user_id, and role claims. Every request carries the JWT in the Authorization header. Middleware validates the token, extracts tenant context, and injects it into the request before any handler executes.

**Why JWT:** The case study specifies a front-end consumer. JWT is stateless (no session store needed), carries tenant and role claims inline (no additional lookup per request), and is the industry standard for SPA-to-API authentication. Token expiry and refresh are straightforward to implement.

**Tenant isolation is enforced at the middleware level**, not at the query level. Every database query is scoped to the authenticated tenant, but the scoping happens in a data access layer that receives tenant context from the middleware — individual handlers never construct tenant-filtered queries directly. This prevents accidental cross-tenant data leakage from a missed WHERE clause.

**For partner/API access (not implemented in case study):** OAuth2 client credentials flow for machine-to-machine authentication. API keys as a simpler alternative for early integrations. Both would flow through the same gateway and tenant-scoping middleware.

**Role-based access control:** The JWT carries role claims (e.g., admin, analyst, viewer). Module-level and action-level permissions are enforced per request. For the case study, this is implemented as middleware that checks role against endpoint requirements. In production, this extends to the entitlements and licensing model described in the platform roadmap.

---

## Task 3: Reliability and Production Readiness

### Input Validation and Schema Drift

Validation is schema-driven, not hardcoded. The resolved schema configuration (global defaults merged with tenant overrides) defines:

- **Required vs. optional columns:** Missing required columns fail the upload. Missing optional columns are noted as warnings but don't block processing.
- **Domain-aware types:** Not just primitives. Types include currency, percentage (0-100), rate, binary flag, geographic identifier, index (bounded numeric range), and population count. Each type carries validation rules — a percentage column rejects values of 250 or -3 at ingest, not when the scoring function produces nonsense downstream.
- **Value ranges and constraints:** Min/max bounds, allowed values for categorical fields, nullability rules.
- **Unexpected columns:** Columns not in the resolved schema are flagged in validation warnings. They're preserved in the JSONB record but not used in scoring unless mapped through a tenant schema update. This allows forward-compatible data ingestion — a customer can send more data than the current model uses without breaking anything.

**Schema drift handling:** When a customer's data changes shape (new columns appear, types change), the validation catches it at ingest and surfaces clear errors. Schema config is versioned — the system can track when a tenant's schema was last updated and flag uploads that don't match the expected shape.

### Safe File Ingestion

- **File type restriction:** CSV only. Content-type validation plus magic byte checking (don't trust the extension alone).
- **Size limits:** Configurable per-tenant, enforced before the file is fully read. Default reasonable limit (e.g., 100MB) with the ability to raise for enterprise tenants.
- **Streaming parse:** CSV is parsed in a streaming fashion — rows are read, validated, and written to the database in batches. A million-row file never sits entirely in memory. Parse errors are collected and reported per-row with line numbers.
- **Temp storage:** Uploaded files land in a temp directory with a short TTL. Once parsed and loaded into the database, the raw file can be retained (for audit/reprocessing) or purged per retention policy.
- **Virus/malware scanning:** *Stub in this implementation.* In production, files pass through a scanning service (ClamAV or cloud-native equivalent such as Google Cloud DLP) before parsing. The insertion point is after upload, before the streaming parse begins. The pipeline blocks until scan completes; a failed scan rejects the upload with a clear error.

### Background Job Execution and Retries

Scoring runs execute asynchronously. The API returns a run ID immediately (202 Accepted) and the scoring work happens in a background goroutine backed by a job status table in Postgres.

**Job lifecycle:** queued → running → succeeded | failed

**Retry policy:**
- **Transient failures retry:** Database timeouts, model endpoint unavailability, temporary resource contention. Retries use exponential backoff with jitter. Maximum retry count is configurable (default: 3).
- **Permanent failures do not retry:** Validation errors, malformed data that passed initial checks, schema mismatches. These fail the run immediately with a descriptive error.
- **Attempt tracking:** Each run records attempt_count, last_error, and timestamps for each attempt. This is visible in the run status endpoint for debugging.

**Why Postgres-backed instead of a dedicated queue:** At this scale, a job table with status polling is simpler to operate, easier to query for debugging, and avoids introducing an additional infrastructure dependency (Redis, RabbitMQ). The status table is also the audit record. In production at higher throughput, this graduates to a proper task queue (Asynq/Redis or Cloud Tasks) with the same interface — the job runner is behind an abstraction that doesn't care where the work comes from.

### Idempotency and Duplicate Suppression

Both upload and scoring run endpoints accept an optional `idempotency_key` header/field.

- The idempotency key is stored alongside the resource record.
- If a request arrives with a key that matches an existing record for the same tenant, the system returns the existing resource (200 for uploads, 200 with existing run_id for scoring runs) instead of creating a duplicate.
- Keys are scoped to tenant — the same key from different tenants creates independent resources.
- Keys expire after a configurable window (e.g., 24 hours) to prevent unbounded storage growth.

This handles the primary use case: flaky networks causing client retries that shouldn't produce duplicate scoring runs.

### Observability

The scoring service emits structured JSON logs with a consistent contract designed for platform-wide observability — not just this one service.

**Every log entry includes:**
```json
{
  "timestamp": "ISO-8601",
  "level": "info|warn|error",
  "service": "scoring-service",
  "instance_id": "uuid",
  "transaction_id": "uuid",
  "tenant_id": "uuid",
  "correlation_id": "uuid",
  "step": "ingest|validate|transform|score|explain|publish",
  "duration_ms": 142,
  "outcome": "success|failure",
  "message": "Human-readable context",
  "metadata": { }
}
```

**Three pillars, one contract:**

- **Tracing:** Instance ID and transaction ID correlate every step of a scoring run. Correlation ID ties back to the originating API request. You can follow a single run from upload through validation through every scored row to the published results.
- **Logging:** Structured JSON at every pipeline step. Configurable verbosity levels — minimal in production for performance, verbose/replay mode when a customer asks "why did site 47 score higher than site 12." Replay mode re-executes the run with debug-level logging against the snapshotted configuration.
- **Metrics:** Derived from the structured logs. Scoring run duration (p50, p95, p99), error rates by tenant, error rates by pipeline step, job queue depth, upload volume and size trends. In production, these feed dashboards and alerting.

**Platform-wide design intent:** This log contract is not specific to the scoring service. It's designed so that any service on the Workforce AI platform — HRIS connectors, partner API handlers, the WBA backend, future agentic orchestration — emits the same structure. The entire platform feeds into a shared observability layer (ELK, Datadog, or GCP Cloud Logging depending on operational preferences). Per-tenant log isolation is handled at the index/filter level, not by running separate infrastructure.

*For this case study, the service outputs structured JSON logs to stdout. A production deployment adds a log shipper (Filebeat) and an aggregation layer (Elasticsearch + Kibana) as additional containers. The log contract is the deliverable; the infrastructure is configuration.*

---

## Task 4: Data Modeling, Performance, and Scaling

### Storage Approach

**Postgres** (Cloud SQL/AlloyDB in production — AlloyDB is Postgres under the hood, consistent with Workforce AI's GCP commitment).

| Data | Storage | Rationale |
|------|---------|-----------|
| Raw uploads (file bytes) | Object storage (GCS in production, local temp dir for case study) | Files can be large; don't store blobs in the database. Retained for audit/reprocessing. |
| Upload metadata | Relational table | Tenant, filename, row count, schema version, validation status, timestamps. Standard indexed queries. |
| Parsed site records | Relational table with JSONB | Fixed columns for identifiers (site_id, tenant_id, upload_id). JSONB column for all data fields. Supports flexible schema without DDL changes per tenant. |
| Schema configurations | Relational table (versioned) | Global defaults and tenant overrides. Each version is immutable once created. |
| Scoring runs | Relational table | Run ID, upload reference, model version, schema config snapshot ID, status, attempt count, timestamps. The orchestration and audit record. |
| Schema config snapshots | Relational table (JSONB) | Full resolved configuration captured at run time. Immutable. Referenced by run ID. |
| Recommendations/results | Relational table with JSONB | Rank, site_id, final_score, raw_score, and a JSONB explanation object. Shape varies per schema config. |

### Indexing and Query Patterns

**"Show me top sites" (ranked recommendations):**
- Composite index on `(run_id, final_score DESC)` supports paginated ranked queries efficiently.
- Additional filter indexes on `(run_id, final_score)` for min_score filtering.

**"What happened in this run?" (audit history):**
- Index on `(tenant_id, created_at DESC)` on scoring_runs for "show me recent runs for this tenant."
- Index on `(upload_id)` on scoring_runs to trace all runs against a given upload.
- Schema config snapshot is retrieved by ID via primary key — no additional index needed.

**"Why did this site score this way?" (single-site detail):**
- Composite index on `(run_id, site_id)` on recommendations for direct lookup.

**Tenant isolation:**
- `tenant_id` is part of every composite index. Queries never scan across tenants. This is enforced at the data access layer, not left to individual queries.

### Scaling to Larger Clients

**1M rows:**
- Streaming CSV parse already handles this — rows are never fully in memory. Parsed in batches (e.g., 1000 rows), validated, and bulk-inserted.
- Scoring processes rows in batches with configurable batch size. Each batch is a single transaction — if a batch fails, only that batch retries, not the entire run.
- JSONB indexing on frequently-queried dynamic fields can be added selectively using Postgres GIN indexes when query patterns stabilize.

**Concurrent runs:**
- Each scoring run is an independent unit of work with its own instance ID, transaction ID, and configuration snapshot. Multiple runs can execute concurrently without interference.
- Connection pooling (pgxpool in Go) prevents connection exhaustion under concurrent load.
- At higher concurrency, the Postgres-backed job runner graduates to a dedicated task queue. The interface doesn't change — the job runner abstraction accepts work and reports status regardless of the backing implementation.

**Multi-tenant at scale:**
- Shared database with tenant-scoped queries (default). For tenants with extreme volume or isolation requirements, the architecture supports tenant-specific database instances — the data access layer resolves the connection based on tenant configuration.
- Read replicas for recommendation queries if read volume outpaces write throughput.

### Trade-offs

| Decision | Trade-off |
|----------|-----------|
| JSONB for dynamic fields | Flexibility over query performance. GIN indexes mitigate, but deeply nested queries will always be slower than fixed columns. Acceptable because schema flexibility is a core product requirement. |
| Postgres-backed job queue | Simplicity over throughput. Works well at current scale, easy to debug, audit-friendly. Graduates to Redis/Cloud Tasks when throughput demands it. |
| Configuration snapshotting | Storage cost over referential simplicity. Every run stores a full copy of the resolved config. At high run volume, this adds storage. Acceptable because auditability is a core platform requirement and configs are small relative to data. |
| Streaming parse with batch insert | Memory efficiency over raw speed. Could be faster with bulk COPY operations, but streaming gives better error reporting per-row and works within memory constraints for very large files. |
| Single service (monolith first) | Deployment simplicity over independent scaling. All endpoints in one Go binary. When specific endpoints need independent scaling (e.g., recommendation reads vs. upload writes), extract into separate services behind the same API gateway. The internal package structure already supports this split. |

---

## Task 5: Communication

### Executive Summary

We built a backend scoring service that lets Workforce AI ingest a client's location data, run it through a configurable scoring pipeline, and deliver ranked site recommendations with clear, auditable explanations of why each site scored the way it did. The system is designed so that every recommendation can be traced back to the exact data, model version, and configuration that produced it — which is essential for enterprise clients who need to justify decisions to their boards and regulators. Results are delivered through a clean API that both the Workforce AI web application and future partner integrations can consume without modification.

This is a foundational service, not a one-off. The schema configuration is hierarchical — global defaults that apply to every client, with per-tenant overrides that allow custom data fields and scoring weights — so onboarding a new customer with different data doesn't require engineering changes. The scoring pipeline is modular, with each step (validate, transform, score, explain, publish) as an independent stage that can be swapped or extended. This means the same architecture supports future models beyond Site Selection IQ, and the same explainability framework works regardless of which model produced the score. The immediate next priority is hardening the ingest pipeline for real-world data quality issues, integrating with a production model service, and validating the tenant isolation model under concurrent load.

### First-Week Follow-Ups After Shipping MVP

1. **Load test with realistic data volumes.** Generate synthetic datasets at 10x and 100x the sample size and run concurrent scoring jobs to identify bottlenecks in the batch processing, database write throughput, and connection pooling. Establish baseline latency numbers (p50, p95) for the scoring pipeline to benchmark future improvements against.

2. **Integrate with the real model service endpoint.** The placeholder scoring function needs to be replaced with a call to the data science team's deployed model. This means aligning on the model service API contract (input features, output schema, versioning), confirming that the structured explanation format captures the factors the real model uses, and validating that the end-to-end pipeline produces correct results against a known test dataset.

3. **Schema configuration validation with a real customer dataset.** Take an actual client's data (anonymized if necessary) and run it through the schema validation and scoring pipeline. Identify gaps between the synthetic test data and real-world data quality — missing values, unexpected formats, columns that don't map cleanly. This is where the flexible schema design gets pressure-tested against reality.

---

## Architectural Note: AI as a Translation Layer

There is an important distinction between marketing a platform as "AI-powered" and actually building sustainable, modular engineering systems. The models are the engine, but they are not the entire car.

The heavy computational work in this platform is done by purpose-built ML models maintained by the data science team. The engineering platform's job is to get the right data to those models, execute them reliably, capture their outputs with full traceability, and present results in a way that humans can understand and act on. AI — specifically large language models — plays a specific, bounded role in this architecture: translating structured model outputs into human-readable narratives. That is a translation layer, not a computational one.

This distinction matters for three reasons:

**Reliability.** The deterministic scoring pipeline produces the same results given the same inputs, every time. It's auditable, reproducible, and explainable without any AI involvement. The LLM narrative enhances the experience but never replaces the underlying data. If the LLM is unavailable, the system degrades gracefully — structured explanations still work perfectly.

**Cost and performance.** LLM inference is expensive and slow relative to a weighted scoring function. By confining the LLM to an optional narrative layer that runs after scoring is complete, we avoid putting expensive AI calls in the critical path of every computation. The platform scales with data volume, not with AI inference costs.

**Trust.** Enterprise customers need to know exactly why a recommendation was made. "The AI said so" is not an acceptable answer for a logistics company deciding where to build a $50M distribution center. A structured explanation that traces back to specific data inputs, specific weights, and a specific model version is. The LLM narrative makes that explanation accessible to non-technical stakeholders, but the trust comes from the deterministic layer underneath.

Building the platform this way — sustainable, modular pipelines with AI as a targeted enhancement rather than a load-bearing wall — is what separates a product that scales from a demo that impresses in a pitch deck.
