package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/workforce-ai/site-selection-iq/internal/models"
)

// RecommendationRepository handles data access for recommendation records
type RecommendationRepository struct {
	pool *pgxpool.Pool
}

// NewRecommendationRepository creates a new recommendation repository
func NewRecommendationRepository(pool *pgxpool.Pool) *RecommendationRepository {
	return &RecommendationRepository{pool: pool}
}

// BulkInsert performs a batch insert of recommendations using parameterized queries
func (r *RecommendationRepository) BulkInsert(ctx context.Context, recs []models.Recommendation) error {
	if len(recs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}

	query := `
		INSERT INTO recommendations (
			id, run_id, tenant_id, site_id, site_name, ranking,
			final_score, component_scores, metadata, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`

	for _, rec := range recs {
		batch.Queue(
			query,
			rec.ID,
			rec.RunID,
			rec.TenantID,
			rec.SiteID,
			rec.SiteName,
			rec.Ranking,
			rec.FinalScore,
			rec.ComponentScores,
			rec.Metadata,
			rec.CreatedAt,
		)
	}

	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()

	for i := 0; i < len(recs); i++ {
		_, err := results.Exec()
		if err != nil {
			return err
		}
	}

	return nil
}

// GetByRun retrieves recommendations for a given run with pagination,
// optionally filtered by minimum score, ordered by final_score DESC
func (r *RecommendationRepository) GetByRun(
	ctx context.Context,
	runID uuid.UUID,
	page int,
	pageSize int,
	minScore *float64,
) ([]models.Recommendation, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize

	// Get total count
	countQuery := `
		SELECT COUNT(*)
		FROM recommendations
		WHERE run_id = $1
	`
	countArgs := []interface{}{runID}

	if minScore != nil {
		countQuery += ` AND final_score >= $2`
		countArgs = append(countArgs, *minScore)
	}

	var totalCount int
	err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	query := `
		SELECT id, run_id, tenant_id, site_id, site_name, ranking,
		       final_score, component_scores, metadata, created_at
		FROM recommendations
		WHERE run_id = $1
	`
	args := []interface{}{runID}

	if minScore != nil {
		query += ` AND final_score >= $2`
		args = append(args, *minScore)
	}

	limitParamNum := len(args) + 1
	offsetParamNum := len(args) + 2
	query += ` ORDER BY final_score DESC
		LIMIT $` + fmt.Sprintf("%d", limitParamNum) + ` OFFSET $` + fmt.Sprintf("%d", offsetParamNum)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var recommendations []models.Recommendation
	for rows.Next() {
		rec := models.Recommendation{}
		err := rows.Scan(
			&rec.ID,
			&rec.RunID,
			&rec.TenantID,
			&rec.SiteID,
			&rec.SiteName,
			&rec.Ranking,
			&rec.FinalScore,
			&rec.ComponentScores,
			&rec.Metadata,
			&rec.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		recommendations = append(recommendations, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return recommendations, totalCount, nil
}

// GetBySiteID retrieves a recommendation for a specific site within a run
func (r *RecommendationRepository) GetBySiteID(ctx context.Context, runID uuid.UUID, siteID string) (*models.Recommendation, error) {
	query := `
		SELECT id, run_id, tenant_id, site_id, site_name, ranking,
		       final_score, component_scores, metadata, created_at
		FROM recommendations
		WHERE run_id = $1 AND site_id = $2
	`

	rec := &models.Recommendation{}
	err := r.pool.QueryRow(ctx, query, runID, siteID).Scan(
		&rec.ID,
		&rec.RunID,
		&rec.TenantID,
		&rec.SiteID,
		&rec.SiteName,
		&rec.Ranking,
		&rec.FinalScore,
		&rec.ComponentScores,
		&rec.Metadata,
		&rec.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return rec, nil
}
