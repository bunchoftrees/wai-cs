package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Tenant represents a platform tenant.
type Tenant struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Slug      string          `json:"slug"`
	Settings  json.RawMessage `json:"settings"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Upload represents an uploaded CSV file.
// DB columns: id, tenant_id, filename, file_size, status, validation_status,
//
//	row_count, schema_version, warnings, errors, idempotency_key, created_at, updated_at
type Upload struct {
	ID               uuid.UUID       `json:"upload_id"`
	TenantID         uuid.UUID       `json:"tenant_id"`
	Filename         string          `json:"filename"`
	FileSize         int64           `json:"file_size"`
	Status           string          `json:"status"`
	ValidationStatus string          `json:"validation_status"`
	RowCount         int             `json:"row_count"`
	SchemaVersion    string          `json:"schema_version"`
	Warnings         json.RawMessage `json:"warnings"`
	Errors           json.RawMessage `json:"errors"`
	IdempotencyKey   *string         `json:"idempotency_key,omitempty"`
	ContentHash      *string         `json:"content_hash,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// SiteRecord represents a parsed CSV row stored as JSONB.
// DB columns: id, upload_id, tenant_id, site_id, site_name, location,
//
//	latitude, longitude, raw_data, data, created_at
type SiteRecord struct {
	ID        uuid.UUID       `json:"id"`
	UploadID  uuid.UUID       `json:"upload_id"`
	TenantID  uuid.UUID       `json:"tenant_id"`
	SiteID    string          `json:"site_id"`
	SiteName  string          `json:"site_name"`
	Location  string          `json:"location"`
	Latitude  *float64        `json:"latitude,omitempty"`
	Longitude *float64        `json:"longitude,omitempty"`
	RawData   json.RawMessage `json:"raw_data"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

// ScoringRun represents an asynchronous scoring execution.
// DB columns: id, upload_id, tenant_id, status, model_version, scoring_config,
//
//	schema_config_snapshot_id, instance_id, transaction_id, row_count,
//	scored_count, attempt, last_error, idempotency_key, duration_ms,
//	started_at, completed_at, created_at, updated_at
type ScoringRun struct {
	ID                     uuid.UUID       `json:"run_id"`
	UploadID               uuid.UUID       `json:"upload_id"`
	TenantID               uuid.UUID       `json:"tenant_id"`
	Status                 string          `json:"status"`
	ModelVersion           string          `json:"model_version"`
	ScoringConfig          json.RawMessage `json:"scoring_config,omitempty"`
	SchemaConfigSnapshotID *uuid.UUID      `json:"schema_config_snapshot_id,omitempty"`
	InstanceID             uuid.UUID       `json:"instance_id"`
	TransactionID          uuid.UUID       `json:"transaction_id"`
	RowCount               *int            `json:"row_count,omitempty"`
	ScoredCount            *int            `json:"scored_count,omitempty"`
	Attempt                int             `json:"attempt"`
	LastError              *string         `json:"last_error,omitempty"`
	IdempotencyKey         *string         `json:"idempotency_key,omitempty"`
	DurationMs             *int            `json:"duration_ms,omitempty"`
	StartedAt              *time.Time      `json:"started_at,omitempty"`
	CompletedAt            *time.Time      `json:"completed_at,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

// Recommendation holds a scored site result with explanation.
// DB columns: id, run_id, tenant_id, site_id, site_name, ranking,
//
//	final_score, component_scores, metadata, created_at
type Recommendation struct {
	ID              uuid.UUID       `json:"id"`
	RunID           uuid.UUID       `json:"run_id"`
	TenantID        uuid.UUID       `json:"tenant_id"`
	SiteID          string          `json:"site_id"`
	SiteName        string          `json:"site_name"`
	Ranking         int             `json:"ranking"`
	FinalScore      float64         `json:"final_score"`
	RawScore        float64         `json:"-"` // computed, not stored
	ComponentScores json.RawMessage `json:"component_scores"`
	Metadata        json.RawMessage `json:"metadata"`
	Explanation     json.RawMessage `json:"-"` // serialized into component_scores
	CreatedAt       time.Time       `json:"created_at"`
}

// SchemaConfig holds schema configuration (global or tenant-specific).
// DB columns: id, tenant_id, version, config, schema_definition, description,
//
//	is_active, created_at, updated_at
type SchemaConfig struct {
	ID               uuid.UUID       `json:"id"`
	TenantID         *uuid.UUID      `json:"tenant_id,omitempty"`
	Version          string          `json:"version"`
	Config           json.RawMessage `json:"config"`
	SchemaDefinition json.RawMessage `json:"schema_definition"`
	Description      string          `json:"description"`
	IsActive         bool            `json:"is_active"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// SchemaConfigSnapshot captures config at scoring time for auditability.
// DB columns: id, run_id, schema_config_id, upload_id, config, snapshot_data, created_at
type SchemaConfigSnapshot struct {
	ID             uuid.UUID       `json:"id"`
	RunID          uuid.UUID       `json:"run_id"`
	SchemaConfigID uuid.UUID       `json:"schema_config_id"`
	UploadID       *uuid.UUID      `json:"upload_id,omitempty"`
	Config         json.RawMessage `json:"config"`
	SnapshotData   json.RawMessage `json:"snapshot_data"`
	CreatedAt      time.Time       `json:"created_at"`
}

// ExplanationFactor describes a single scoring factor's contribution.
type ExplanationFactor struct {
	Name         string  `json:"name"`
	Value        float64 `json:"value"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
	Direction    string  `json:"direction"`
	Reason       string  `json:"reason"`
}

// Explanation contains the full structured explanation for a recommendation.
type Explanation struct {
	Factors []ExplanationFactor `json:"factors"`
	Summary string              `json:"summary"`
}

// Pagination holds pagination metadata.
type Pagination struct {
	Page         int `json:"page"`
	PageSize     int `json:"page_size"`
	TotalResults int `json:"total_results"`
	TotalPages   int `json:"total_pages"`
}
