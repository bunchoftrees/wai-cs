package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/workforce-ai/site-selection-iq/internal/models"
)

// SiteRecordRepository handles data access for site records parsed from CSV uploads
type SiteRecordRepository struct {
	pool *pgxpool.Pool
}

// NewSiteRecordRepository creates a new site record repository
func NewSiteRecordRepository(pool *pgxpool.Pool) *SiteRecordRepository {
	return &SiteRecordRepository{pool: pool}
}

// BulkInsert performs a batch insert of site records using parameterized queries
func (r *SiteRecordRepository) BulkInsert(ctx context.Context, records []models.SiteRecord) error {
	if len(records) == 0 {
		return nil
	}

	batch := &pgx.Batch{}

	query := `
		INSERT INTO site_records (
			id, upload_id, tenant_id, site_id, site_name, location,
			latitude, longitude, raw_data, data, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`

	for _, record := range records {
		batch.Queue(
			query,
			record.ID,
			record.UploadID,
			record.TenantID,
			record.SiteID,
			record.SiteName,
			record.Location,
			record.Latitude,
			record.Longitude,
			record.RawData,
			record.Data,
			record.CreatedAt,
		)
	}

	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()

	for i := 0; i < len(records); i++ {
		_, err := results.Exec()
		if err != nil {
			return err
		}
	}

	return nil
}

// GetByUpload retrieves all site records for a given upload
func (r *SiteRecordRepository) GetByUpload(ctx context.Context, uploadID uuid.UUID) ([]models.SiteRecord, error) {
	query := `
		SELECT id, upload_id, tenant_id, site_id, site_name, location,
		       latitude, longitude, raw_data, data, created_at
		FROM site_records
		WHERE upload_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, uploadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.SiteRecord
	for rows.Next() {
		record := models.SiteRecord{}
		err := rows.Scan(
			&record.ID,
			&record.UploadID,
			&record.TenantID,
			&record.SiteID,
			&record.SiteName,
			&record.Location,
			&record.Latitude,
			&record.Longitude,
			&record.RawData,
			&record.Data,
			&record.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

// CountByUpload returns the total number of site records for a given upload
func (r *SiteRecordRepository) CountByUpload(ctx context.Context, uploadID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM site_records
		WHERE upload_id = $1
	`

	var count int
	err := r.pool.QueryRow(ctx, query, uploadID).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
