package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/workforce-ai/site-selection-iq/internal/api/response"
	"github.com/workforce-ai/site-selection-iq/internal/models"
	"github.com/workforce-ai/site-selection-iq/internal/repository"
)

// RecommendationHandler handles recommendation and explanation endpoints.
type RecommendationHandler struct {
	recommendationRepo *repository.RecommendationRepository
	runRepo            *repository.RunRepository
	schemaConfigRepo   *repository.SchemaConfigRepository
}

// NewRecommendationHandler creates a new recommendation handler.
func NewRecommendationHandler(
	recommendationRepo *repository.RecommendationRepository,
	runRepo *repository.RunRepository,
	schemaConfigRepo *repository.SchemaConfigRepository,
) *RecommendationHandler {
	return &RecommendationHandler{
		recommendationRepo: recommendationRepo,
		runRepo:            runRepo,
		schemaConfigRepo:   schemaConfigRepo,
	}
}

// HandleGetRecommendations handles GET /api/v1/runs/:run_id/recommendations.
func (h *RecommendationHandler) HandleGetRecommendations(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	// Parse run_id from URL
	runIDStr := c.Param("run_id")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		response.BadRequest(c, "invalid run_id format", nil)
		return
	}

	// Parse pagination params
	page := 1
	pageSize := 20

	if pageParam := c.Query("page"); pageParam != "" {
		var p int
		if _, err := fmt.Sscanf(pageParam, "%d", &p); err == nil && p > 0 {
			page = p
		}
	}

	if pageSizeParam := c.Query("page_size"); pageSizeParam != "" {
		var ps int
		if _, err := fmt.Sscanf(pageSizeParam, "%d", &ps); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// Parse optional min_score filter
	var minScore *float64
	if minScoreParam := c.Query("min_score"); minScoreParam != "" {
		var ms float64
		if _, err := fmt.Sscanf(minScoreParam, "%f", &ms); err == nil && ms >= 0 {
			minScore = &ms
		}
	}

	// Verify run exists and belongs to tenant
	run, err := h.runRepo.GetByID(c.Request.Context(), tenantID, runID)
	if err != nil {
		response.InternalError(c, fmt.Sprintf("failed to retrieve run: %v", err))
		return
	}
	if run == nil {
		response.NotFound(c, "run not found")
		return
	}

	// Get paginated recommendations
	recommendations, totalCount, err := h.recommendationRepo.GetByRun(
		c.Request.Context(),
		runID,
		page,
		pageSize,
		minScore,
	)
	if err != nil {
		response.InternalError(c, fmt.Sprintf("failed to retrieve recommendations: %v", err))
		return
	}

	// Build recommendation response objects with inline explanations
	recResponses := make([]gin.H, len(recommendations))
	for i, rec := range recommendations {
		// Parse component_scores into explanation
		var explanation models.Explanation
		if len(rec.ComponentScores) > 0 {
			_ = json.Unmarshal(rec.ComponentScores, &explanation)
		}

		// Extract raw_score from metadata
		var rawScore float64
		if len(rec.Metadata) > 0 {
			var meta map[string]interface{}
			if json.Unmarshal(rec.Metadata, &meta) == nil {
				if rs, ok := meta["raw_score"].(float64); ok {
					rawScore = rs
				}
			}
		}

		recResponses[i] = gin.H{
			"rank":        rec.Ranking,
			"site_id":     rec.SiteID,
			"site_name":   rec.SiteName,
			"final_score": rec.FinalScore,
			"raw_score":   rawScore,
			"explanation": explanation,
		}
	}

	// Build pagination metadata
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}

	pagination := models.Pagination{
		Page:         page,
		PageSize:     pageSize,
		TotalResults: totalCount,
		TotalPages:   totalPages,
	}

	result := gin.H{
		"run_id":          runID,
		"recommendations": recResponses,
		"pagination":      pagination,
	}

	response.Success(c, http.StatusOK, result)
}

// HandleGetExplanation handles GET /api/v1/runs/:run_id/recommendations/:site_id/explain.
func (h *RecommendationHandler) HandleGetExplanation(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	// Parse run_id from URL
	runIDStr := c.Param("run_id")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		response.BadRequest(c, "invalid run_id format", nil)
		return
	}

	siteID := c.Param("site_id")

	// Verify run exists and belongs to tenant
	run, err := h.runRepo.GetByID(c.Request.Context(), tenantID, runID)
	if err != nil {
		response.InternalError(c, fmt.Sprintf("failed to retrieve run: %v", err))
		return
	}
	if run == nil {
		response.NotFound(c, "run not found")
		return
	}

	// Get recommendation by run_id + site_id
	rec, err := h.recommendationRepo.GetBySiteID(c.Request.Context(), runID, siteID)
	if err != nil {
		response.InternalError(c, fmt.Sprintf("failed to retrieve recommendation: %v", err))
		return
	}
	if rec == nil {
		response.NotFound(c, "recommendation not found")
		return
	}

	// Parse component_scores as explanation factors
	var explanation models.Explanation
	if len(rec.ComponentScores) > 0 {
		_ = json.Unmarshal(rec.ComponentScores, &explanation)
	}

	// Extract raw_score from metadata
	var rawScore float64
	if len(rec.Metadata) > 0 {
		var meta map[string]interface{}
		if json.Unmarshal(rec.Metadata, &meta) == nil {
			if rs, ok := meta["raw_score"].(float64); ok {
				rawScore = rs
			}
		}
	}

	// Build weights_applied from schema config snapshot (if available)
	var weightsApplied gin.H
	if run.SchemaConfigSnapshotID != nil {
		snapshot, err := h.schemaConfigRepo.GetSnapshot(c.Request.Context(), *run.SchemaConfigSnapshotID)
		if err == nil && snapshot != nil {
			// Parse snapshot_data to extract the weight_set
			var snapshotData map[string]interface{}
			if json.Unmarshal(snapshot.SnapshotData, &snapshotData) == nil {
				weightSet := make(map[string]interface{})
				if weights, ok := snapshotData["weights"].(map[string]interface{}); ok {
					weightSet = weights
				}
				weightsApplied = gin.H{
					"source":                    "tenant_override",
					"schema_config_snapshot_id": snapshot.ID,
					"weight_set":               weightSet,
				}
			}
		}
	}

	// Parse optional include_narrative query param
	includeNarrative := c.Query("include_narrative") == "true"

	// Build explanation response matching case study spec
	explanationObj := gin.H{
		"factors":       explanation.Factors,
		"summary":       explanation.Summary,
		"model_version": run.ModelVersion,
	}
	if run.CompletedAt != nil {
		explanationObj["scored_at"] = run.CompletedAt
	}
	if weightsApplied != nil {
		explanationObj["weights_applied"] = weightsApplied
	}

	result := gin.H{
		"site_id":     rec.SiteID,
		"site_name":   rec.SiteName,
		"run_id":      rec.RunID,
		"final_score": rec.FinalScore,
		"raw_score":   rawScore,
		"explanation": explanationObj,
	}

	// Add narrative stub if requested (per case study: "may implement if time permits")
	if includeNarrative {
		result["narrative"] = "LLM narrative generation is stubbed. " +
			"In production, this would contain an AI-generated natural-language explanation " +
			"of the recommendation based on the structured explanation factors. " +
			"See explanation.factors for the auditable deterministic source."
		result["narrative_metadata"] = gin.H{
			"generated_by": "stub",
			"model":        "none",
			"source":       "deterministic_explanation",
			"disclaimer":   "Narrative generated from structured scoring data. See explanation.factors for auditable source.",
		}
	}

	response.Success(c, http.StatusOK, result)
}
