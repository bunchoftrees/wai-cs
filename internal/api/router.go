package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/workforce-ai/site-selection-iq/internal/api/handlers"
	"github.com/workforce-ai/site-selection-iq/internal/api/middleware"
	"github.com/workforce-ai/site-selection-iq/internal/config"
	"github.com/workforce-ai/site-selection-iq/internal/repository"
	"github.com/workforce-ai/site-selection-iq/internal/schema"
	"github.com/workforce-ai/site-selection-iq/internal/scoring"
	"github.com/workforce-ai/site-selection-iq/pkg/auth"
)

// NewRouter creates and configures the Gin router with all routes and middleware.
func NewRouter(pool *pgxpool.Pool, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Global middleware
	r.Use(gin.Recovery())
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.CorrelationMiddleware())
	r.Use(middleware.StructuredLogging())

	// Health check (no auth required)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"service": "site-selection-iq",
		})
	})

	// Initialize repositories
	uploadRepo := repository.NewUploadRepository(pool)
	siteRecordRepo := repository.NewSiteRecordRepository(pool)
	runRepo := repository.NewRunRepository(pool)
	recRepo := repository.NewRecommendationRepository(pool)
	schemaConfigRepo := repository.NewSchemaConfigRepository(pool)
	idempotencyRepo := repository.NewIdempotencyRepository(pool)

	// Initialize services
	schemaResolver := schema.NewResolver()
	scoreFn := scoring.DefaultScoreFunc

	// Initialize scoring pipeline
	pipeline := scoring.NewPipeline(
		runRepo,
		siteRecordRepo,
		recRepo,
		schemaConfigRepo,
		schemaResolver,
		scoreFn,
		cfg.Scoring.MaxRetries,
		cfg.Scoring.RetryBaseWait,
	)

	// Initialize handlers
	uploadHandler := handlers.NewUploadHandler(uploadRepo, siteRecordRepo, schemaConfigRepo, idempotencyRepo, schemaResolver, cfg)
	runHandler := handlers.NewRunHandler(runRepo, uploadRepo, idempotencyRepo, pipeline, cfg)
	recHandler := handlers.NewRecommendationHandler(recRepo, runRepo, schemaConfigRepo)

	// API v1 routes (authenticated)
	v1 := r.Group("/api/v1")
	v1.Use(middleware.AuthMiddleware(&cfg.JWT))
	{
		// Uploads — require admin or analyst role
		v1.POST("/uploads",
			middleware.RequireRole("admin", "analyst"),
			uploadHandler.HandleUpload,
		)

		// Scoring runs — require admin or analyst role
		v1.POST("/uploads/:upload_id/runs",
			middleware.RequireRole("admin", "analyst"),
			runHandler.HandleCreateRun,
		)
		v1.GET("/runs/:run_id",
			middleware.RequireRole("admin", "analyst", "viewer"),
			runHandler.HandleGetRun,
		)

		// Recommendations — all authenticated roles can view
		v1.GET("/runs/:run_id/recommendations",
			middleware.RequireRole("admin", "analyst", "viewer"),
			recHandler.HandleGetRecommendations,
		)
		v1.GET("/runs/:run_id/recommendations/:site_id/explain",
			middleware.RequireRole("admin", "analyst", "viewer"),
			recHandler.HandleGetExplanation,
		)
	}

	// Token generation endpoint (dev only — generates test JWTs)
	r.POST("/dev/token", devTokenHandler(cfg))

	// Serve static demo frontend and Swagger UI
	r.Static("/static", "./static")
	r.StaticFile("/openapi.yaml", "./openapi.yaml")
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/static/index.html")
	})
	r.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/static/swagger.html")
	})

	return r
}

// devTokenHandler returns a handler that generates test JWTs for development.
func devTokenHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			TenantID string `json:"tenant_id"`
			UserID   string `json:"user_id"`
			Role     string `json:"role"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}

		tenantID, err := uuid.Parse(req.TenantID)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid tenant_id"})
			return
		}
		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid user_id"})
			return
		}
		if req.Role == "" {
			req.Role = "admin"
		}

		token, err := auth.GenerateToken(cfg.JWT.Secret, cfg.JWT.Issuer, tenantID, userID, req.Role, cfg.JWT.ExpiryHours)
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to generate token"})
			return
		}

		c.JSON(200, gin.H{"token": token})
	}
}
