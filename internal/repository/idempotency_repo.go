package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IdempotencyResult holds the outcome of an atomic claim attempt.
type IdempotencyResult struct {
	// AlreadyExists is true when the key was already claimed.
	AlreadyExists bool
	// ResourceID is the resource_id associated with the key (existing or newly claimed).
	ResourceID uuid.UUID
}

// IdempotencyRepository handles atomic idempotency key operations.
type IdempotencyRepository struct {
	pool *pgxpool.Pool
}

// NewIdempotencyRepository creates a new idempotency repository.
func NewIdempotencyRepository(pool *pgxpool.Pool) *IdempotencyRepository {
	return &IdempotencyRepository{pool: pool}
}

// Claim atomically attempts to claim an idempotency key for a resource.
// If the key already exists (same tenant + key + resource_type), it returns
// AlreadyExists=true with the original resource_id. If not, it inserts the
// key and returns AlreadyExists=false. This is race-condition-safe because
// it uses INSERT ... ON CONFLICT with the composite primary key.
func (r *IdempotencyRepository) Claim(
	ctx context.Context,
	tenantID uuid.UUID,
	key string,
	resourceType string,
	resourceID uuid.UUID,
) (*IdempotencyResult, error) {
	if key == "" {
		return nil, errors.New("idempotency key cannot be empty")
	}

	// Atomic upsert: try to insert; on conflict, return the existing row.
	// The idempotency_keys table PK is (tenant_id, key, resource_type).
	query := `
		WITH inserted AS (
			INSERT INTO idempotency_keys (key, tenant_id, resource_type, resource_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (tenant_id, key, resource_type) DO NOTHING
			RETURNING resource_id, FALSE AS already_exists
		)
		SELECT resource_id, already_exists FROM inserted
		UNION ALL
		SELECT resource_id, TRUE AS already_exists
		FROM idempotency_keys
		WHERE tenant_id = $2 AND key = $1 AND resource_type = $3
		  AND NOT EXISTS (SELECT 1 FROM inserted)
	`

	var result IdempotencyResult
	err := r.pool.QueryRow(ctx, query, key, tenantID, resourceType, resourceID).Scan(
		&result.ResourceID,
		&result.AlreadyExists,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Should not happen with the UNION ALL pattern, but handle gracefully
			return nil, errors.New("unexpected empty result from idempotency claim")
		}
		return nil, err
	}

	return &result, nil
}

// CleanExpired removes expired idempotency keys. Call from a background job.
func (r *IdempotencyRepository) CleanExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
