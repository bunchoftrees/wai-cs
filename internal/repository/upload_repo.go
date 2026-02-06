package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/workforce-ai/site-selection-iq/internal/models"
)

// UploadRepository handles data access for upload records
type UploadRepository struct {
	pool *pgxpool.Pool
}

// NewUploadRepository creates a new upload repository
func NewUploadRepository(pool *pgxpool.Pool) *UploadRepository {
	return &UploadRepository{pool: pool}
}

// uploadColumns is the canonical column list for uploads, used across all queries.
const uploadColumns = `id, tenant_id, filename, file_size, status, validation_status,
	row_count, schema_version, warnings, errors, idempotency_key, content_hash,
	created_at, updated_at`

// scanUpload scans a row into an Upload struct using the canonical column order.
func scanUpload(row pgx.Row, upload *models.Upload) error {
	return row.Scan(
		&upload.ID,
		&upload.TenantID,
		&upload.Filename,
		&upload.FileSize,
		&upload.Status,
		&upload.ValidationStatus,
		&upload.RowCount,
		&upload.SchemaVersion,
		&upload.Warnings,
		&upload.Errors,
		&upload.IdempotencyKey,
		&upload.ContentHash,
		&upload.CreatedAt,
		&upload.UpdatedAt,
	)
}

// Create inserts a new upload record
func (r *UploadRepository) Create(ctx context.Context, upload *models.Upload) error {
	if upload == nil {
		return errors.New("upload cannot be nil")
	}

	query := `
		INSERT INTO uploads (
			id, tenant_id, filename, file_size, status, validation_status,
			row_count, schema_version, warnings, errors, idempotency_key, content_hash,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
		RETURNING ` + uploadColumns

	return scanUpload(r.pool.QueryRow(
		ctx, query,
		upload.ID, upload.TenantID, upload.Filename, upload.FileSize,
		upload.Status, upload.ValidationStatus, upload.RowCount, upload.SchemaVersion,
		upload.Warnings, upload.Errors, upload.IdempotencyKey, upload.ContentHash,
		upload.CreatedAt, upload.UpdatedAt,
	), upload)
}

// GetByID retrieves an upload by ID, scoped to the tenant
func (r *UploadRepository) GetByID(ctx context.Context, tenantID, uploadID uuid.UUID) (*models.Upload, error) {
	query := `SELECT ` + uploadColumns + ` FROM uploads WHERE id = $1 AND tenant_id = $2`
	upload := &models.Upload{}
	err := scanUpload(r.pool.QueryRow(ctx, query, uploadID, tenantID), upload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return upload, nil
}

// GetByIdempotencyKey retrieves an upload by idempotency key, scoped to the tenant
func (r *UploadRepository) GetByIdempotencyKey(ctx context.Context, tenantID uuid.UUID, key string) (*models.Upload, error) {
	query := `SELECT ` + uploadColumns + ` FROM uploads WHERE tenant_id = $1 AND idempotency_key = $2`
	upload := &models.Upload{}
	err := scanUpload(r.pool.QueryRow(ctx, query, tenantID, key), upload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return upload, nil
}

// GetByContentHash retrieves an upload by SHA-256 content hash, scoped to the tenant.
// Returns nil, nil if no match found.
func (r *UploadRepository) GetByContentHash(ctx context.Context, tenantID uuid.UUID, hash string) (*models.Upload, error) {
	query := `SELECT ` + uploadColumns + ` FROM uploads WHERE tenant_id = $1 AND content_hash = $2`
	upload := &models.Upload{}
	err := scanUpload(r.pool.QueryRow(ctx, query, tenantID, hash), upload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return upload, nil
}

// Update updates an upload record
func (r *UploadRepository) Update(ctx context.Context, upload *models.Upload) error {
	if upload == nil {
		return errors.New("upload cannot be nil")
	}

	query := `
		UPDATE uploads
		SET filename = $3, file_size = $4,
		    status = $5, validation_status = $6, row_count = $7,
		    schema_version = $8, warnings = $9, errors = $10,
		    idempotency_key = $11, content_hash = $12, updated_at = $13
		WHERE id = $1 AND tenant_id = $2
		RETURNING ` + uploadColumns

	err := scanUpload(r.pool.QueryRow(
		ctx, query,
		upload.ID, upload.TenantID, upload.Filename, upload.FileSize,
		upload.Status, upload.ValidationStatus, upload.RowCount, upload.SchemaVersion,
		upload.Warnings, upload.Errors, upload.IdempotencyKey, upload.ContentHash,
		upload.UpdatedAt,
	), upload)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("upload not found")
		}
		return err
	}
	return nil
}
