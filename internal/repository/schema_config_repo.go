package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/workforce-ai/site-selection-iq/internal/models"
)

// SchemaConfigRepository handles data access for schema configuration records
type SchemaConfigRepository struct {
	pool *pgxpool.Pool
}

// NewSchemaConfigRepository creates a new schema config repository
func NewSchemaConfigRepository(pool *pgxpool.Pool) *SchemaConfigRepository {
	return &SchemaConfigRepository{pool: pool}
}

// GetGlobalActive retrieves the currently active global schema configuration
func (r *SchemaConfigRepository) GetGlobalActive(ctx context.Context) (*models.SchemaConfig, error) {
	query := `
		SELECT id, tenant_id, version, config, schema_definition, description,
		       is_active, created_at, updated_at
		FROM schema_configs
		WHERE tenant_id IS NULL AND is_active = true
		ORDER BY version DESC
		LIMIT 1
	`

	config := &models.SchemaConfig{}
	err := r.pool.QueryRow(ctx, query).Scan(
		&config.ID,
		&config.TenantID,
		&config.Version,
		&config.Config,
		&config.SchemaDefinition,
		&config.Description,
		&config.IsActive,
		&config.CreatedAt,
		&config.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return config, nil
}

// GetTenantActive retrieves the currently active schema configuration for a specific tenant
func (r *SchemaConfigRepository) GetTenantActive(ctx context.Context, tenantID uuid.UUID) (*models.SchemaConfig, error) {
	query := `
		SELECT id, tenant_id, version, config, schema_definition, description,
		       is_active, created_at, updated_at
		FROM schema_configs
		WHERE tenant_id = $1 AND is_active = true
		ORDER BY version DESC
		LIMIT 1
	`

	config := &models.SchemaConfig{}
	err := r.pool.QueryRow(ctx, query, tenantID).Scan(
		&config.ID,
		&config.TenantID,
		&config.Version,
		&config.Config,
		&config.SchemaDefinition,
		&config.Description,
		&config.IsActive,
		&config.CreatedAt,
		&config.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return config, nil
}

// CreateSnapshot creates a new schema configuration snapshot
func (r *SchemaConfigRepository) CreateSnapshot(ctx context.Context, snapshot *models.SchemaConfigSnapshot) error {
	if snapshot == nil {
		return errors.New("schema config snapshot cannot be nil")
	}

	query := `
		INSERT INTO schema_config_snapshots (
			id, run_id, schema_config_id, upload_id, config, snapshot_data,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
		RETURNING id, run_id, schema_config_id, upload_id, config, snapshot_data,
		          created_at
	`

	err := r.pool.QueryRow(
		ctx,
		query,
		snapshot.ID,
		snapshot.RunID,
		snapshot.SchemaConfigID,
		snapshot.UploadID,
		snapshot.Config,
		snapshot.SnapshotData,
		snapshot.CreatedAt,
	).Scan(
		&snapshot.ID,
		&snapshot.RunID,
		&snapshot.SchemaConfigID,
		&snapshot.UploadID,
		&snapshot.Config,
		&snapshot.SnapshotData,
		&snapshot.CreatedAt,
	)

	if err != nil {
		return err
	}

	return nil
}

// GetSnapshot retrieves a specific schema configuration snapshot by ID
func (r *SchemaConfigRepository) GetSnapshot(ctx context.Context, snapshotID uuid.UUID) (*models.SchemaConfigSnapshot, error) {
	query := `
		SELECT id, run_id, schema_config_id, upload_id, config, snapshot_data,
		       created_at
		FROM schema_config_snapshots
		WHERE id = $1
	`

	snapshot := &models.SchemaConfigSnapshot{}
	err := r.pool.QueryRow(ctx, query, snapshotID).Scan(
		&snapshot.ID,
		&snapshot.RunID,
		&snapshot.SchemaConfigID,
		&snapshot.UploadID,
		&snapshot.Config,
		&snapshot.SnapshotData,
		&snapshot.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return snapshot, nil
}
