-- 001_initial_schema.sql
-- Site Selection IQ - Initial database schema
-- Designed for multi-tenant, schema-driven scoring pipeline

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================
-- Tenants
-- ============================================================
CREATE TABLE IF NOT EXISTS tenants (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    settings    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- Schema Configurations (hierarchical: global + tenant overrides)
-- ============================================================
CREATE TABLE IF NOT EXISTS schema_configs (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id         UUID REFERENCES tenants(id),  -- NULL = global default
    version           TEXT NOT NULL,
    config            JSONB NOT NULL,
    schema_definition JSONB DEFAULT '{}',
    description       TEXT DEFAULT '',
    is_active         BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, version)
);

CREATE INDEX IF NOT EXISTS idx_schema_configs_tenant_active ON schema_configs (tenant_id, is_active) WHERE is_active = true;

-- ============================================================
-- Uploads
-- ============================================================
CREATE TABLE IF NOT EXISTS uploads (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id),
    filename            TEXT NOT NULL DEFAULT '',
    file_size           BIGINT NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'pending',
    validation_status   TEXT NOT NULL DEFAULT 'pending'
                        CHECK (validation_status IN ('pending', 'valid', 'invalid')),
    row_count           INTEGER,
    schema_version      TEXT,
    warnings            JSONB DEFAULT '[]',
    errors              JSONB DEFAULT '[]',
    idempotency_key     TEXT,
    content_hash        TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_uploads_tenant ON uploads (tenant_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_uploads_idempotency ON uploads (tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_uploads_content_hash ON uploads (tenant_id, content_hash) WHERE content_hash IS NOT NULL;

-- ============================================================
-- Site Records (parsed CSV rows with JSONB dynamic fields)
-- ============================================================
CREATE TABLE IF NOT EXISTS site_records (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    upload_id   UUID NOT NULL REFERENCES uploads(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    site_id     TEXT NOT NULL,
    site_name   TEXT DEFAULT '',
    location    TEXT DEFAULT '',
    latitude    DOUBLE PRECISION,
    longitude   DOUBLE PRECISION,
    raw_data    JSONB DEFAULT '{}',
    data        JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_site_records_upload ON site_records (upload_id);
CREATE INDEX IF NOT EXISTS idx_site_records_tenant_site ON site_records (tenant_id, site_id);

-- ============================================================
-- Scoring Runs
-- ============================================================
CREATE TABLE IF NOT EXISTS scoring_runs (
    id                          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    upload_id                   UUID NOT NULL REFERENCES uploads(id),
    tenant_id                   UUID NOT NULL REFERENCES tenants(id),
    status                      TEXT NOT NULL DEFAULT 'queued'
                                CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'completed')),
    model_version               TEXT NOT NULL DEFAULT 'site-selection-iq-v1.0',
    scoring_config              JSONB DEFAULT '{}',
    schema_config_snapshot_id   UUID,
    instance_id                 UUID NOT NULL DEFAULT uuid_generate_v4(),
    transaction_id              UUID NOT NULL DEFAULT uuid_generate_v4(),
    row_count                   INTEGER,
    scored_count                INTEGER,
    attempt                     INTEGER NOT NULL DEFAULT 0,
    last_error                  TEXT,
    idempotency_key             TEXT,
    duration_ms                 INTEGER,
    started_at                  TIMESTAMPTZ,
    completed_at                TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scoring_runs_tenant ON scoring_runs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_scoring_runs_upload ON scoring_runs (upload_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_scoring_runs_idempotency ON scoring_runs (tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

-- ============================================================
-- Schema Config Snapshots (immutable, captured at run time)
-- ============================================================
CREATE TABLE IF NOT EXISTS schema_config_snapshots (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    run_id            UUID NOT NULL REFERENCES scoring_runs(id) ON DELETE CASCADE,
    schema_config_id  UUID DEFAULT uuid_generate_v4(),
    upload_id         UUID,
    config            JSONB NOT NULL DEFAULT '{}',
    snapshot_data     JSONB DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- Recommendations (scored results with structured explanations)
-- ============================================================
CREATE TABLE IF NOT EXISTS recommendations (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    run_id           UUID NOT NULL REFERENCES scoring_runs(id) ON DELETE CASCADE,
    tenant_id        UUID NOT NULL REFERENCES tenants(id),
    site_id          TEXT NOT NULL,
    site_name        TEXT DEFAULT '',
    ranking          INTEGER NOT NULL DEFAULT 0,
    final_score      NUMERIC(10,4) NOT NULL,
    component_scores JSONB DEFAULT '{}',
    metadata         JSONB DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recommendations_run_score ON recommendations (run_id, final_score DESC);
CREATE INDEX IF NOT EXISTS idx_recommendations_run_site ON recommendations (run_id, site_id);
CREATE INDEX IF NOT EXISTS idx_recommendations_tenant ON recommendations (tenant_id);

-- ============================================================
-- Idempotency Keys (for expiration tracking)
-- ============================================================
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key           TEXT NOT NULL,
    tenant_id     UUID NOT NULL REFERENCES tenants(id),
    resource_type TEXT NOT NULL,
    resource_id   UUID NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '24 hours'),
    PRIMARY KEY (tenant_id, key, resource_type)
);

-- ============================================================
-- Seed: Global schema configuration
-- ============================================================
INSERT INTO schema_configs (id, tenant_id, version, config) VALUES (
    uuid_generate_v4(),
    NULL,  -- global default
    'v1.0',
    '{
        "fields": {
            "site_id": {
                "type": "identifier",
                "required": true,
                "description": "Unique site identifier"
            },
            "city": {
                "type": "text",
                "required": false,
                "description": "City name"
            },
            "state": {
                "type": "text",
                "required": false,
                "description": "State/province"
            },
            "unemployment_rate": {
                "type": "percentage",
                "required": true,
                "min": 0,
                "max": 100,
                "weight": 1.0,
                "direction": "maximize",
                "description": "Local unemployment rate — higher suggests available labor pool"
            },
            "labor_cost_index": {
                "type": "index",
                "required": true,
                "min": 0,
                "max": 200,
                "weight": 0.5,
                "direction": "minimize",
                "description": "Labor cost index relative to national average (100 = average)"
            },
            "working_age_pop": {
                "type": "population",
                "required": true,
                "min": 0,
                "weight": 1.0,
                "direction": "maximize",
                "description": "Working-age population (18-65) in the area"
            },
            "local_competitors": {
                "type": "integer",
                "required": true,
                "min": 0,
                "weight": 0.8,
                "direction": "minimize",
                "description": "Number of competing employers in the area"
            },
            "avg_commute_time": {
                "type": "numeric",
                "required": false,
                "min": 0,
                "max": 180,
                "weight": 0.3,
                "direction": "minimize",
                "description": "Average commute time in minutes"
            },
            "public_transport_access": {
                "type": "percentage",
                "required": false,
                "min": 0,
                "max": 100,
                "weight": 0.6,
                "direction": "maximize",
                "description": "Percentage of area served by public transit"
            },
            "cost_of_living_index": {
                "type": "index",
                "required": false,
                "min": 0,
                "max": 300,
                "weight": 0.4,
                "direction": "minimize",
                "description": "Cost of living index (100 = national average)"
            },
            "education_rate": {
                "type": "percentage",
                "required": false,
                "min": 0,
                "max": 100,
                "weight": 0.5,
                "direction": "maximize",
                "description": "Percentage of population with post-secondary education"
            }
        },
        "site_id_column": "site_id",
        "defaults": {
            "model_version": "site-selection-iq-v1.0"
        }
    }'::jsonb
) ON CONFLICT DO NOTHING;

-- ============================================================
-- Seed: Demo tenant
-- ============================================================
INSERT INTO tenants (id, name, slug) VALUES (
    '550e8400-e29b-41d4-a716-446655440000',
    'Acme Logistics',
    'acme-logistics'
) ON CONFLICT DO NOTHING;

-- Tenant-specific schema override (adds warehouse_sq_footage, adjusts weights)
INSERT INTO schema_configs (tenant_id, version, config) VALUES (
    '550e8400-e29b-41d4-a716-446655440000',
    'v1.0',
    '{
        "fields": {
            "warehouse_sq_footage": {
                "type": "numeric",
                "required": false,
                "min": 0,
                "weight": 0.3,
                "direction": "maximize",
                "description": "Available warehouse square footage"
            }
        },
        "weights": {
            "unemployment_rate": 1.3,
            "local_competitors": 0.8,
            "labor_cost_index": 0.5,
            "avg_commute_time": 0.3,
            "working_age_pop": 1.0,
            "public_transport_access": 0.6
        }
    }'::jsonb
) ON CONFLICT DO NOTHING;

-- ============================================================
-- Seed: Second demo tenant (case-study prompt column layout)
-- Demonstrates schema-driven approach: different CSV shape,
-- zero code changes — only a schema override is needed.
-- ============================================================
INSERT INTO tenants (id, name, slug) VALUES (
    '660e8400-e29b-41d4-a716-446655440000',
    'Globex Distribution',
    'globex-distribution'
) ON CONFLICT DO NOTHING;

-- Tenant override adds the three columns unique to the case-study prompt
-- (model_score_raw, median_household_income, successful_site) and redefines
-- public_transport_access as binary (0/1) instead of a percentage.
INSERT INTO schema_configs (tenant_id, version, config) VALUES (
    '660e8400-e29b-41d4-a716-446655440000',
    'v1.0',
    '{
        "fields": {
            "model_score_raw": {
                "type": "numeric",
                "required": true,
                "min": 0,
                "max": 100,
                "weight": 1.5,
                "direction": "maximize",
                "description": "Pre-computed model score (0-100) from upstream ML pipeline"
            },
            "median_household_income": {
                "type": "numeric",
                "required": true,
                "min": 0,
                "weight": 0.7,
                "direction": "maximize",
                "description": "Median household income in the site area (USD)"
            },
            "successful_site": {
                "type": "integer",
                "required": true,
                "min": 0,
                "max": 1,
                "weight": 0.0,
                "direction": "maximize",
                "description": "Binary label — 1 if site was historically successful (ground truth, excluded from scoring)"
            },
            "public_transport_access": {
                "type": "integer",
                "required": false,
                "min": 0,
                "max": 1,
                "weight": 0.6,
                "direction": "maximize",
                "description": "Binary — 1 if public transport is accessible near the site"
            }
        },
        "weights": {
            "unemployment_rate": 1.0,
            "labor_cost_index": 0.5,
            "working_age_pop": 1.0,
            "local_competitors": 0.8,
            "avg_commute_time": 0.3,
            "model_score_raw": 1.5,
            "median_household_income": 0.7,
            "public_transport_access": 0.6,
            "successful_site": 0.0
        }
    }'::jsonb
) ON CONFLICT DO NOTHING;
