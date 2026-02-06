package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/workforce-ai/site-selection-iq/internal/models"
	"github.com/workforce-ai/site-selection-iq/internal/repository"
	"github.com/workforce-ai/site-selection-iq/internal/schema"
)

// Pipeline manages the asynchronous scoring execution workflow.
// It coordinates between repositories, schema resolution, and the scoring function.
type Pipeline struct {
	runRepo            *repository.RunRepository
	siteRecordRepo     *repository.SiteRecordRepository
	recommendationRepo *repository.RecommendationRepository
	schemaConfigRepo   *repository.SchemaConfigRepository
	schemaResolver     *schema.Resolver
	scoreFunc          ScoreFunc
	maxRetries         int
	retryBaseWait      time.Duration
}

// NewPipeline creates a new scoring pipeline
func NewPipeline(
	runRepo *repository.RunRepository,
	siteRecordRepo *repository.SiteRecordRepository,
	recommendationRepo *repository.RecommendationRepository,
	schemaConfigRepo *repository.SchemaConfigRepository,
	schemaResolver *schema.Resolver,
	scoreFunc ScoreFunc,
	maxRetries int,
	retryBaseWait time.Duration,
) *Pipeline {
	if scoreFunc == nil {
		scoreFunc = DefaultScoreFunc
	}
	return &Pipeline{
		runRepo:            runRepo,
		siteRecordRepo:     siteRecordRepo,
		recommendationRepo: recommendationRepo,
		schemaConfigRepo:   schemaConfigRepo,
		schemaResolver:     schemaResolver,
		scoreFunc:          scoreFunc,
		maxRetries:         maxRetries,
		retryBaseWait:      retryBaseWait,
	}
}

// Execute performs the synchronous scoring pipeline execution.
// Steps:
// a. Updates run status to "running"
// b. Resolves schema config (global + tenant)
// c. Creates schema config snapshot
// d. Fetches site records for the upload
// e. Scores each site using the ScoreFunc
// f. Ranks results by final_score DESC
// g. Bulk inserts recommendations
// h. Updates run status to "succeeded" with duration_ms and scored_count
// On error: updates run status to "failed" with last_error
func (p *Pipeline) Execute(ctx context.Context, run *models.ScoringRun) error {
	startTime := time.Now()
	logger := slog.Default().With(
		slog.String("service", "scoring-pipeline"),
		slog.String("instance_id", run.InstanceID.String()),
		slog.String("transaction_id", run.TransactionID.String()),
		slog.String("tenant_id", run.TenantID.String()),
		slog.String("run_id", run.ID.String()),
	)

	// Step a: Update run status to "running"
	stepLogger := logger.With(slog.String("step", "update_status_running"))
	stepLogger.Info("updating run status to running")

	if err := p.runRepo.UpdateStatus(ctx, run.ID, "running", nil, nil, nil); err != nil {
		stepLogger.Error("failed to update run status", slog.String("error", err.Error()))
		return p.handleExecutionError(ctx, logger, run, err)
	}

	// Step b: Resolve schema config (global + tenant)
	stepLogger = logger.With(slog.String("step", "resolve_schema_config"))
	stepLogger.Info("resolving schema configuration")

	globalConfig, err := p.schemaConfigRepo.GetGlobalActive(ctx)
	if err != nil {
		stepLogger.Error("failed to get global schema config", slog.String("error", err.Error()))
		return p.handleExecutionError(ctx, logger, run, err)
	}

	if globalConfig == nil {
		err := fmt.Errorf("no active global schema configuration found")
		stepLogger.Error("schema configuration missing", slog.String("error", err.Error()))
		return p.handleExecutionError(ctx, logger, run, err)
	}

	tenantConfig, err := p.schemaConfigRepo.GetTenantActive(ctx, run.TenantID)
	if err != nil {
		stepLogger.Error("failed to get tenant schema config", slog.String("error", err.Error()))
		return p.handleExecutionError(ctx, logger, run, err)
	}

	// Resolve schema with tenant overrides via the resolver instance
	var tenantConfigBytes json.RawMessage
	if tenantConfig != nil {
		tenantConfigBytes = tenantConfig.Config
	}

	resolvedSchema, err := p.schemaResolver.Resolve(ctx, globalConfig.Config, tenantConfigBytes)
	if err != nil {
		stepLogger.Error("failed to resolve schema", slog.String("error", err.Error()))
		return p.handleExecutionError(ctx, logger, run, err)
	}

	stepLogger.Info("schema resolved successfully",
		slog.Int("field_count", len(resolvedSchema.Fields)))

	// Step c: Create schema config snapshot
	stepLogger = logger.With(slog.String("step", "create_snapshot"))
	stepLogger.Info("creating schema config snapshot")

	snapshotID := uuid.New()

	// Serialize resolved schema as snapshot data
	snapshotData, err := json.Marshal(resolvedSchema)
	if err != nil {
		stepLogger.Error("failed to marshal snapshot data", slog.String("error", err.Error()))
		return p.handleExecutionError(ctx, logger, run, err)
	}

	snapshot := &models.SchemaConfigSnapshot{
		ID:             snapshotID,
		RunID:          run.ID,
		SchemaConfigID: globalConfig.ID,
		UploadID:       &run.UploadID,
		Config:         globalConfig.Config,
		SnapshotData:   snapshotData,
		CreatedAt:      time.Now(),
	}

	if err := p.schemaConfigRepo.CreateSnapshot(ctx, snapshot); err != nil {
		stepLogger.Error("failed to create snapshot", slog.String("error", err.Error()))
		return p.handleExecutionError(ctx, logger, run, err)
	}

	// Update run with snapshot ID
	run.SchemaConfigSnapshotID = &snapshotID

	stepLogger.Info("snapshot created", slog.String("snapshot_id", snapshotID.String()))

	// Step d: Fetch site records for the upload
	stepLogger = logger.With(slog.String("step", "fetch_site_records"))
	stepLogger.Info("fetching site records for upload")

	siteRecords, err := p.siteRecordRepo.GetByUpload(ctx, run.UploadID)
	if err != nil {
		stepLogger.Error("failed to fetch site records", slog.String("error", err.Error()))
		return p.handleExecutionError(ctx, logger, run, err)
	}

	stepLogger.Info("site records fetched", slog.Int("count", len(siteRecords)))

	if len(siteRecords) == 0 {
		// Update run status to succeeded with 0 scored count
		completeDuration := int(time.Since(startTime).Milliseconds())
		if err := p.runRepo.UpdateStatus(ctx, run.ID, "succeeded", intPtr(0), nil, intPtr(completeDuration)); err != nil {
			logger.Error("failed to update final status", slog.String("error", err.Error()))
		}
		return nil
	}

	// Step e & f: Score each site and collect results
	stepLogger = logger.With(slog.String("step", "score_sites"))
	stepLogger.Info("scoring sites")

	recommendations := make([]models.Recommendation, 0, len(siteRecords))

	for idx, siteRecord := range siteRecords {
		// Parse site data from JSON
		var siteData map[string]interface{}
		if err := json.Unmarshal(siteRecord.Data, &siteData); err != nil {
			stepLogger.Warn("failed to parse site data, skipping site",
				slog.String("site_id", siteRecord.SiteID),
				slog.String("error", err.Error()))
			continue
		}

		// Score the site
		rawScore, finalScore, explanation, err := p.scoreFunc(siteData, resolvedSchema)
		if err != nil {
			stepLogger.Warn("failed to score site, skipping",
				slog.String("site_id", siteRecord.SiteID),
				slog.String("error", err.Error()))
			continue
		}

		// Serialize explanation to JSON — stored in component_scores DB column
		explanationJSON, err := json.Marshal(explanation)
		if err != nil {
			stepLogger.Warn("failed to marshal explanation, using empty",
				slog.String("site_id", siteRecord.SiteID))
			explanationJSON = []byte("{}")
		}

		// Build metadata with raw score info
		metadataJSON, _ := json.Marshal(map[string]interface{}{
			"raw_score":    rawScore,
			"model_version": run.ModelVersion,
		})

		// Create recommendation (Ranking set to 0, will be assigned after sorting)
		rec := models.Recommendation{
			ID:              uuid.New(),
			RunID:           run.ID,
			TenantID:        run.TenantID,
			SiteID:          siteRecord.SiteID,
			SiteName:        siteRecord.SiteName,
			Ranking:         0,
			FinalScore:      finalScore,
			RawScore:        rawScore,
			ComponentScores: explanationJSON,
			Metadata:        metadataJSON,
			CreatedAt:       time.Now(),
		}

		recommendations = append(recommendations, rec)

		if (idx+1)%100 == 0 {
			stepLogger.Info("scoring progress",
				slog.Int("scored", idx+1),
				slog.Int("total", len(siteRecords)))
		}
	}

	stepLogger.Info("sites scored",
		slog.Int("scored_count", len(recommendations)),
		slog.Int("total_count", len(siteRecords)))

	// Sort recommendations by final_score DESC and assign rankings
	sortRecommendationsByScore(recommendations)
	for i := range recommendations {
		recommendations[i].Ranking = i + 1
	}

	// Step g: Bulk insert recommendations
	stepLogger = logger.With(slog.String("step", "bulk_insert_recommendations"))
	stepLogger.Info("bulk inserting recommendations")

	if err := p.recommendationRepo.BulkInsert(ctx, recommendations); err != nil {
		stepLogger.Error("failed to bulk insert recommendations", slog.String("error", err.Error()))
		return p.handleExecutionError(ctx, logger, run, err)
	}

	stepLogger.Info("recommendations inserted", slog.Int("count", len(recommendations)))

	// Step h: Update run status to "succeeded"
	stepLogger = logger.With(slog.String("step", "update_status_succeeded"))
	stepLogger.Info("updating run status to succeeded")

	completeDuration := int(time.Since(startTime).Milliseconds())
	scoredCount := len(recommendations)

	if err := p.runRepo.UpdateStatus(ctx, run.ID, "succeeded", intPtr(scoredCount), nil, intPtr(completeDuration)); err != nil {
		stepLogger.Error("failed to update final status", slog.String("error", err.Error()))
		return err
	}

	logger.Info("scoring pipeline completed successfully",
		slog.Int("duration_ms", completeDuration),
		slog.Int("scored_count", scoredCount))

	return nil
}

// ExecuteWithRetry wraps Execute with exponential backoff + jitter retry logic
func (p *Pipeline) ExecuteWithRetry(ctx context.Context, run *models.ScoringRun) error {
	logger := slog.Default().With(
		slog.String("service", "scoring-pipeline"),
		slog.String("instance_id", run.InstanceID.String()),
		slog.String("transaction_id", run.TransactionID.String()),
		slog.String("tenant_id", run.TenantID.String()),
		slog.String("run_id", run.ID.String()),
	)

	var lastErr error

	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		logger.Info("executing scoring pipeline",
			slog.Int("attempt", attempt+1),
			slog.Int("max_retries", p.maxRetries))

		// Increment attempt counter
		if err := p.runRepo.IncrementAttempt(ctx, run.ID); err != nil {
			logger.Error("failed to increment attempt counter", slog.String("error", err.Error()))
		}

		// Try to execute the pipeline
		err := p.Execute(ctx, run)
		if err == nil {
			logger.Info("scoring pipeline succeeded")
			return nil
		}

		lastErr = err
		logger.Warn("scoring pipeline failed",
			slog.String("error", err.Error()),
			slog.Int("attempt", attempt+1))

		// Don't retry if we've exhausted retries
		if attempt >= p.maxRetries {
			break
		}

		// Calculate exponential backoff with jitter
		backoff := p.calculateBackoff(attempt)

		logger.Info("retrying after backoff",
			slog.Int("backoff_ms", int(backoff.Milliseconds())),
			slog.Int("next_attempt", attempt+2))

		// Wait before retry
		select {
		case <-time.After(backoff):
			// Continue to next retry
		case <-ctx.Done():
			logger.Info("context cancelled, stopping retries")
			return ctx.Err()
		}
	}

	// All retries exhausted
	errorMsg := fmt.Sprintf("scoring pipeline failed after %d attempts: %v", p.maxRetries+1, lastErr)
	logger.Error("all retry attempts exhausted", slog.String("error", errorMsg))

	if err := p.runRepo.UpdateStatus(ctx, run.ID, "failed", nil, stringPtr(errorMsg), nil); err != nil {
		logger.Error("failed to update failed status", slog.String("error", err.Error()))
	}

	return fmt.Errorf("%s", errorMsg)
}

// calculateBackoff calculates exponential backoff with jitter
// Formula: min(baseWait * 2^attempt + random jitter, maxWait)
func (p *Pipeline) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: base * 2^attempt
	exponentialMs := p.retryBaseWait.Milliseconds() * int64(math.Pow(2, float64(attempt)))

	// Add jitter: random value between 0 and exponentialMs * 0.1
	jitterMs := rand.Int63n(exponentialMs/10 + 1)

	totalMs := exponentialMs + jitterMs

	// Cap at reasonable maximum (5 minutes)
	maxMs := int64(5 * 60 * 1000)
	if totalMs > maxMs {
		totalMs = maxMs
	}

	return time.Duration(totalMs) * time.Millisecond
}

// handleExecutionError updates run status to "failed" and returns the error
func (p *Pipeline) handleExecutionError(
	ctx context.Context,
	logger *slog.Logger,
	run *models.ScoringRun,
	err error,
) error {
	errorMsg := err.Error()
	logger.Error("execution error occurred", slog.String("error", errorMsg))

	if updateErr := p.runRepo.UpdateStatus(ctx, run.ID, "failed", nil, stringPtr(errorMsg), nil); updateErr != nil {
		logger.Error("failed to update run status to failed",
			slog.String("update_error", updateErr.Error()))
	}

	return err
}

// sortRecommendationsByScore sorts recommendations by final_score in descending order
func sortRecommendationsByScore(recommendations []models.Recommendation) {
	for i := 0; i < len(recommendations); i++ {
		for j := i + 1; j < len(recommendations); j++ {
			if recommendations[j].FinalScore > recommendations[i].FinalScore {
				recommendations[i], recommendations[j] = recommendations[j], recommendations[i]
			}
		}
	}
}

// Helper functions for pointer creation
func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}
