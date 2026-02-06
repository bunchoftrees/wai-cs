package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/workforce-ai/site-selection-iq/internal/models"
)

// RunRepository handles data access for scoring run records
type RunRepository struct {
	pool *pgxpool.Pool
}

// NewRunRepository creates a new run repository
func NewRunRepository(pool *pgxpool.Pool) *RunRepository {
	return &RunRepository{pool: pool}
}

// Create inserts a new scoring run record
func (r *RunRepository) Create(ctx context.Context, run *models.ScoringRun) error {
	if run == nil {
		return errors.New("scoring run cannot be nil")
	}

	query := `
		INSERT INTO scoring_runs (
			id, upload_id, tenant_id, status, model_version, scoring_config,
			schema_config_snapshot_id, instance_id, transaction_id, row_count,
			scored_count, attempt, last_error, idempotency_key, duration_ms,
			started_at, completed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
		)
		RETURNING id, upload_id, tenant_id, status, model_version, scoring_config,
		          schema_config_snapshot_id, instance_id, transaction_id, row_count,
		          scored_count, attempt, last_error, idempotency_key, duration_ms,
		          started_at, completed_at, created_at, updated_at
	`

	err := r.pool.QueryRow(
		ctx,
		query,
		run.ID,
		run.UploadID,
		run.TenantID,
		run.Status,
		run.ModelVersion,
		run.ScoringConfig,
		run.SchemaConfigSnapshotID,
		run.InstanceID,
		run.TransactionID,
		run.RowCount,
		run.ScoredCount,
		run.Attempt,
		run.LastError,
		run.IdempotencyKey,
		run.DurationMs,
		run.StartedAt,
		run.CompletedAt,
		run.CreatedAt,
		run.UpdatedAt,
	).Scan(
		&run.ID,
		&run.UploadID,
		&run.TenantID,
		&run.Status,
		&run.ModelVersion,
		&run.ScoringConfig,
		&run.SchemaConfigSnapshotID,
		&run.InstanceID,
		&run.TransactionID,
		&run.RowCount,
		&run.ScoredCount,
		&run.Attempt,
		&run.LastError,
		&run.IdempotencyKey,
		&run.DurationMs,
		&run.StartedAt,
		&run.CompletedAt,
		&run.CreatedAt,
		&run.UpdatedAt,
	)

	if err != nil {
		return err
	}

	return nil
}

// GetByID retrieves a scoring run by ID, scoped to the tenant
func (r *RunRepository) GetByID(ctx context.Context, tenantID, runID uuid.UUID) (*models.ScoringRun, error) {
	query := `
		SELECT id, upload_id, tenant_id, status, model_version, scoring_config,
		       schema_config_snapshot_id, instance_id, transaction_id, row_count,
		       scored_count, attempt, last_error, idempotency_key, duration_ms,
		       started_at, completed_at, created_at, updated_at
		FROM scoring_runs
		WHERE id = $1 AND tenant_id = $2
	`

	run := &models.ScoringRun{}
	err := r.pool.QueryRow(ctx, query, runID, tenantID).Scan(
		&run.ID,
		&run.UploadID,
		&run.TenantID,
		&run.Status,
		&run.ModelVersion,
		&run.ScoringConfig,
		&run.SchemaConfigSnapshotID,
		&run.InstanceID,
		&run.TransactionID,
		&run.RowCount,
		&run.ScoredCount,
		&run.Attempt,
		&run.LastError,
		&run.IdempotencyKey,
		&run.DurationMs,
		&run.StartedAt,
		&run.CompletedAt,
		&run.CreatedAt,
		&run.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return run, nil
}

// GetByIdempotencyKey retrieves a scoring run by idempotency key, scoped to the tenant
func (r *RunRepository) GetByIdempotencyKey(ctx context.Context, tenantID uuid.UUID, key string) (*models.ScoringRun, error) {
	query := `
		SELECT id, upload_id, tenant_id, status, model_version, scoring_config,
		       schema_config_snapshot_id, instance_id, transaction_id, row_count,
		       scored_count, attempt, last_error, idempotency_key, duration_ms,
		       started_at, completed_at, created_at, updated_at
		FROM scoring_runs
		WHERE tenant_id = $1 AND idempotency_key = $2
	`

	run := &models.ScoringRun{}
	err := r.pool.QueryRow(ctx, query, tenantID, key).Scan(
		&run.ID,
		&run.UploadID,
		&run.TenantID,
		&run.Status,
		&run.ModelVersion,
		&run.ScoringConfig,
		&run.SchemaConfigSnapshotID,
		&run.InstanceID,
		&run.TransactionID,
		&run.RowCount,
		&run.ScoredCount,
		&run.Attempt,
		&run.LastError,
		&run.IdempotencyKey,
		&run.DurationMs,
		&run.StartedAt,
		&run.CompletedAt,
		&run.CreatedAt,
		&run.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return run, nil
}

// UpdateStatus updates the status and related fields for a scoring run
func (r *RunRepository) UpdateStatus(
	ctx context.Context,
	runID uuid.UUID,
	status string,
	scoredCount *int,
	lastError *string,
	durationMs *int,
) error {
	query := `
		UPDATE scoring_runs
		SET status = $1,
		    scored_count = COALESCE($2, scored_count),
		    last_error = COALESCE($3, last_error),
		    duration_ms = COALESCE($4, duration_ms),
		    completed_at = CASE WHEN $1 IN ('succeeded', 'failed') THEN NOW() ELSE completed_at END,
		    updated_at = NOW()
		WHERE id = $5
		RETURNING id
	`

	var id uuid.UUID
	err := r.pool.QueryRow(
		ctx,
		query,
		status,
		scoredCount,
		lastError,
		durationMs,
		runID,
	).Scan(&id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("scoring run not found")
		}
		return err
	}

	return nil
}

// Update updates a scoring run record
func (r *RunRepository) Update(ctx context.Context, run *models.ScoringRun) error {
	if run == nil {
		return errors.New("scoring run cannot be nil")
	}

	query := `
		UPDATE scoring_runs
		SET id = $1, upload_id = $2, tenant_id = $3, status = $4, model_version = $5,
		    scoring_config = $6, schema_config_snapshot_id = $7, instance_id = $8,
		    transaction_id = $9, row_count = $10, scored_count = $11, attempt = $12,
		    last_error = $13, idempotency_key = $14, duration_ms = $15,
		    started_at = $16, completed_at = $17, updated_at = $18
		WHERE id = $1
		RETURNING id, upload_id, tenant_id, status, model_version, scoring_config,
		          schema_config_snapshot_id, instance_id, transaction_id, row_count,
		          scored_count, attempt, last_error, idempotency_key, duration_ms,
		          started_at, completed_at, created_at, updated_at
	`

	err := r.pool.QueryRow(
		ctx,
		query,
		run.ID,
		run.UploadID,
		run.TenantID,
		run.Status,
		run.ModelVersion,
		run.ScoringConfig,
		run.SchemaConfigSnapshotID,
		run.InstanceID,
		run.TransactionID,
		run.RowCount,
		run.ScoredCount,
		run.Attempt,
		run.LastError,
		run.IdempotencyKey,
		run.DurationMs,
		run.StartedAt,
		run.CompletedAt,
		run.UpdatedAt,
	).Scan(
		&run.ID,
		&run.UploadID,
		&run.TenantID,
		&run.Status,
		&run.ModelVersion,
		&run.ScoringConfig,
		&run.SchemaConfigSnapshotID,
		&run.InstanceID,
		&run.TransactionID,
		&run.RowCount,
		&run.ScoredCount,
		&run.Attempt,
		&run.LastError,
		&run.IdempotencyKey,
		&run.DurationMs,
		&run.StartedAt,
		&run.CompletedAt,
		&run.CreatedAt,
		&run.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("scoring run not found")
		}
		return err
	}

	return nil
}

// IncrementAttempt increments the attempt counter for a scoring run
func (r *RunRepository) IncrementAttempt(ctx context.Context, runID uuid.UUID) error {
	query := `
		UPDATE scoring_runs
		SET attempt = attempt + 1,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id
	`

	var id uuid.UUID
	err := r.pool.QueryRow(ctx, query, runID).Scan(&id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("scoring run not found")
		}
		return err
	}

	return nil
}
