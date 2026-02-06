package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/workforce-ai/site-selection-iq/internal/api/response"
	"github.com/workforce-ai/site-selection-iq/internal/config"
	"github.com/workforce-ai/site-selection-iq/internal/models"
	"github.com/workforce-ai/site-selection-iq/internal/repository"
	"github.com/workforce-ai/site-selection-iq/internal/scoring"
)

// RunHandler handles scoring run operations.
type RunHandler struct {
	runRepo         *repository.RunRepository
	uploadRepo      *repository.UploadRepository
	idempotencyRepo *repository.IdempotencyRepository
	pipeline        *scoring.Pipeline
	cfg             *config.Config
}

// NewRunHandler creates a new run handler.
func NewRunHandler(
	runRepo *repository.RunRepository,
	uploadRepo *repository.UploadRepository,
	idempotencyRepo *repository.IdempotencyRepository,
	pipeline *scoring.Pipeline,
	cfg *config.Config,
) *RunHandler {
	return &RunHandler{
		runRepo:         runRepo,
		uploadRepo:      uploadRepo,
		idempotencyRepo: idempotencyRepo,
		pipeline:        pipeline,
		cfg:             cfg,
	}
}

// createRunRequest matches the case study's POST body for triggering a scoring run.
type createRunRequest struct {
	IdempotencyKey string          `json:"idempotency_key"`
	ScoringConfig  json.RawMessage `json:"scoring_config"`
}

// HandleCreateRun handles POST /api/v1/uploads/:upload_id/runs.
func (h *RunHandler) HandleCreateRun(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	// Parse upload_id from URL
	uploadIDStr := c.Param("upload_id")
	uploadID, err := uuid.Parse(uploadIDStr)
	if err != nil {
		response.BadRequest(c, "invalid upload_id format", nil)
		return
	}

	// Parse optional request body (scoring_config + idempotency_key)
	var req createRunRequest
	_ = c.ShouldBindJSON(&req) // optional body; OK if missing

	// Determine idempotency key: header takes precedence, then body
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = req.IdempotencyKey
	}

	// Verify upload exists and belongs to tenant
	upload, err := h.uploadRepo.GetByID(c.Request.Context(), tenantID, uploadID)
	if err != nil {
		response.InternalError(c, fmt.Sprintf("failed to retrieve upload: %v", err))
		return
	}
	if upload == nil {
		response.NotFound(c, "upload not found")
		return
	}

	// Verify upload passed validation (422 per spec)
	if upload.ValidationStatus != "valid" {
		response.Error(c, http.StatusUnprocessableEntity, "UNPROCESSABLE",
			"upload failed validation and cannot be scored", nil)
		return
	}

	// Atomic idempotency claim — return 409 Conflict with existing run per spec
	runID := uuid.New()
	if idempotencyKey != "" {
		claim, err := h.idempotencyRepo.Claim(c.Request.Context(), tenantID, idempotencyKey, "scoring_run", runID)
		if err != nil {
			response.InternalError(c, fmt.Sprintf("idempotency check failed: %v", err))
			return
		}
		if claim.AlreadyExists {
			existing, _ := h.runRepo.GetByID(c.Request.Context(), tenantID, claim.ResourceID)
			response.Conflict(c, "duplicate scoring run (idempotency key match)", existing)
			return
		}
	}

	// Create scoring run record
	now := time.Now()
	var idempotencyKeyPtr *string
	if idempotencyKey != "" {
		idempotencyKeyPtr = &idempotencyKey
	}

	// Extract model_version from scoring_config if provided, else use default
	modelVersion := "site-selection-iq-v1.0"
	var scoringConfig json.RawMessage
	if len(req.ScoringConfig) > 0 {
		scoringConfig = req.ScoringConfig
		// Try to extract model_version from request
		var sc struct {
			ModelVersion string `json:"model_version"`
		}
		if json.Unmarshal(req.ScoringConfig, &sc) == nil && sc.ModelVersion != "" {
			if sc.ModelVersion != "latest" {
				modelVersion = sc.ModelVersion
			}
		}
	}

	rowCount := upload.RowCount

	run := &models.ScoringRun{
		ID:             runID,
		UploadID:       uploadID,
		TenantID:       tenantID,
		Status:         "queued",
		ModelVersion:   modelVersion,
		ScoringConfig:  scoringConfig,
		InstanceID:     uuid.New(),
		TransactionID:  uuid.New(),
		RowCount:       &rowCount,
		Attempt:        0,
		IdempotencyKey: idempotencyKeyPtr,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := h.runRepo.Create(c.Request.Context(), run); err != nil {
		response.InternalError(c, fmt.Sprintf("failed to create run: %v", err))
		return
	}

	// Launch scoring pipeline asynchronously
	go func() {
		bgCtx := context.Background()
		_ = h.pipeline.ExecuteWithRetry(bgCtx, run)
	}()

	response.Success(c, http.StatusAccepted, run)
}

// HandleGetRun handles GET /api/v1/runs/:run_id.
func (h *RunHandler) HandleGetRun(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	runIDStr := c.Param("run_id")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		response.BadRequest(c, "invalid run_id format", nil)
		return
	}

	run, err := h.runRepo.GetByID(c.Request.Context(), tenantID, runID)
	if err != nil {
		response.InternalError(c, fmt.Sprintf("failed to retrieve run: %v", err))
		return
	}
	if run == nil {
		response.NotFound(c, "run not found")
		return
	}

	response.Success(c, http.StatusOK, run)
}
