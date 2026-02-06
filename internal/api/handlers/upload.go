package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/workforce-ai/site-selection-iq/internal/api/response"
	"github.com/workforce-ai/site-selection-iq/internal/config"
	"github.com/workforce-ai/site-selection-iq/internal/ingest"
	"github.com/workforce-ai/site-selection-iq/internal/models"
	"github.com/workforce-ai/site-selection-iq/internal/repository"
	"github.com/workforce-ai/site-selection-iq/internal/schema"
)

// UploadHandler handles CSV file uploads.
type UploadHandler struct {
	uploadRepo       *repository.UploadRepository
	siteRecordRepo   *repository.SiteRecordRepository
	schemaConfigRepo *repository.SchemaConfigRepository
	idempotencyRepo  *repository.IdempotencyRepository
	schemaResolver   *schema.Resolver
	cfg              *config.Config
}

// NewUploadHandler creates a new upload handler.
func NewUploadHandler(
	uploadRepo *repository.UploadRepository,
	siteRecordRepo *repository.SiteRecordRepository,
	schemaConfigRepo *repository.SchemaConfigRepository,
	idempotencyRepo *repository.IdempotencyRepository,
	schemaResolver *schema.Resolver,
	cfg *config.Config,
) *UploadHandler {
	return &UploadHandler{
		uploadRepo:       uploadRepo,
		siteRecordRepo:   siteRecordRepo,
		schemaConfigRepo: schemaConfigRepo,
		idempotencyRepo:  idempotencyRepo,
		schemaResolver:   schemaResolver,
		cfg:              cfg,
	}
}

// HandleUpload handles POST /api/v1/uploads.
func (h *UploadHandler) HandleUpload(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	// Check idempotency key atomically — return 409 Conflict with existing upload per spec
	idempotencyKey := c.GetHeader("Idempotency-Key")
	uploadID := uuid.New()
	if idempotencyKey != "" {
		claim, err := h.idempotencyRepo.Claim(c.Request.Context(), tenantID, idempotencyKey, "upload", uploadID)
		if err != nil {
			response.InternalError(c, fmt.Sprintf("idempotency check failed: %v", err))
			return
		}
		if claim.AlreadyExists {
			existing, _ := h.uploadRepo.GetByID(c.Request.Context(), tenantID, claim.ResourceID)
			response.Conflict(c, "duplicate upload (idempotency key match)", existing)
			return
		}
	}

	// Get file from multipart form
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "file field is required", nil)
		return
	}

	// Validate file type (content-type + extension)
	if file.Header.Get("Content-Type") != "text/csv" && filepath.Ext(file.Filename) != ".csv" {
		response.BadRequest(c, "file must be a CSV", nil)
		return
	}

	// Validate file size (413 per spec)
	if file.Size > h.cfg.Upload.MaxFileSize {
		response.Error(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE",
			fmt.Sprintf("file exceeds max size of %d bytes", h.cfg.Upload.MaxFileSize), nil)
		return
	}

	// Open uploaded file
	src, err := file.Open()
	if err != nil {
		response.InternalError(c, "failed to open uploaded file")
		return
	}
	defer src.Close()

	// Save to temp directory
	if err := os.MkdirAll(h.cfg.Upload.TempDir, 0755); err != nil {
		response.InternalError(c, "failed to create temp directory")
		return
	}

	tempPath := filepath.Join(h.cfg.Upload.TempDir, uploadID.String()+".csv")
	tempFile, err := os.Create(tempPath)
	if err != nil {
		response.InternalError(c, "failed to create temp file")
		return
	}
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, src); err != nil {
		os.Remove(tempPath)
		response.InternalError(c, "failed to save file")
		return
	}

	// Compute SHA-256 content hash for deduplication
	hashFile, err := os.Open(tempPath)
	if err != nil {
		os.Remove(tempPath)
		response.InternalError(c, "failed to reopen file for hashing")
		return
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, hashFile); err != nil {
		hashFile.Close()
		os.Remove(tempPath)
		response.InternalError(c, "failed to hash file")
		return
	}
	hashFile.Close()
	contentHash := hex.EncodeToString(hasher.Sum(nil))

	// Check for duplicate content within this tenant — if the same file
	// was already uploaded, return the existing upload (and its results)
	// instead of creating a new one.
	existing, err := h.uploadRepo.GetByContentHash(c.Request.Context(), tenantID, contentHash)
	if err == nil && existing != nil {
		os.Remove(tempPath)
		response.Success(c, http.StatusOK, gin.H{
			"upload_id":         existing.ID,
			"tenant_id":         existing.TenantID,
			"filename":          existing.Filename,
			"row_count":         existing.RowCount,
			"schema_version":    existing.SchemaVersion,
			"validation_status": existing.ValidationStatus,
			"content_hash":      existing.ContentHash,
			"created_at":        existing.CreatedAt,
			"duplicate":         true,
			"message":           "File already uploaded; returning existing upload and its results.",
		})
		return
	}

	// ──────────────────────────────────────────────────────────────────
	// VIRUS / MALWARE SCAN INSERTION POINT
	//
	// In production, insert a call to the malware scanning service here,
	// AFTER the file has been saved to temp storage and BEFORE CSV parsing
	// begins. The scan blocks until completion; a failed scan rejects the
	// upload with a 400 error:
	//
	//   scanResult, err := malwareScanner.Scan(tempPath)
	//   if err != nil || !scanResult.Clean {
	//       os.Remove(tempPath)
	//       response.BadRequest(c, "file rejected by security scan", scanResult.Details)
	//       return
	//   }
	//
	// Recommended integration: ClamAV (self-hosted) or Google Cloud DLP
	// (managed). The scanning interface should accept an io.Reader or file
	// path and return (clean bool, details []string, err error).
	// ──────────────────────────────────────────────────────────────────

	// Create upload record
	now := time.Now()
	var idempotencyKeyPtr *string
	if idempotencyKey != "" {
		idempotencyKeyPtr = &idempotencyKey
	}
	contentHashPtr := &contentHash

	upload := &models.Upload{
		ID:               uploadID,
		TenantID:         tenantID,
		Filename:         file.Filename,
		FileSize:         file.Size,
		Status:           "pending",
		ValidationStatus: "pending",
		RowCount:         0,
		Warnings:         json.RawMessage("[]"),
		Errors:           json.RawMessage("[]"),
		IdempotencyKey:   idempotencyKeyPtr,
		ContentHash:      contentHashPtr,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := h.uploadRepo.Create(c.Request.Context(), upload); err != nil {
		os.Remove(tempPath)
		response.InternalError(c, fmt.Sprintf("failed to create upload record: %v", err))
		return
	}

	// Resolve schema
	globalConfig, err := h.schemaConfigRepo.GetGlobalActive(c.Request.Context())
	if err != nil || globalConfig == nil {
		globalConfig = &models.SchemaConfig{
			Config: json.RawMessage(`{"fields":{"site_id":{"type":"identifier","required":true}},"site_id_column":"site_id"}`),
		}
	}

	tenantConfig, _ := h.schemaConfigRepo.GetTenantActive(c.Request.Context(), tenantID)
	var tenantConfigBytes json.RawMessage
	if tenantConfig != nil {
		tenantConfigBytes = tenantConfig.Config
	}

	resolvedSchema, err := h.schemaResolver.Resolve(c.Request.Context(), globalConfig.Config, tenantConfigBytes)
	if err != nil {
		os.Remove(tempPath)
		response.InternalError(c, fmt.Sprintf("failed to resolve schema: %v", err))
		return
	}

	// Reopen file for parsing
	csvFile, err := os.Open(tempPath)
	if err != nil {
		response.InternalError(c, "failed to reopen file for parsing")
		return
	}
	defer csvFile.Close()

	// Parse and validate CSV
	records, parseWarnings, err := ingest.Parse(csvFile, resolvedSchema)
	if err != nil {
		os.Remove(tempPath)
		upload.ValidationStatus = "invalid"
		upload.UpdatedAt = time.Now()
		_ = h.uploadRepo.Update(c.Request.Context(), upload)
		response.BadRequest(c, fmt.Sprintf("CSV validation failed: %v", err), nil)
		return
	}

	// Capture validation warnings
	warningsJSON, _ := json.Marshal(parseWarnings)
	upload.Warnings = warningsJSON

	// Build site records
	siteRecords := make([]models.SiteRecord, len(records))
	for i, recordData := range records {
		var dataMap map[string]interface{}
		siteID := fmt.Sprintf("row_%d", i+1)
		var siteName, location string
		if err := json.Unmarshal(recordData, &dataMap); err == nil {
			// Extract site_id from the schema-configured column
			if sid, ok := dataMap[resolvedSchema.SiteIDColumn]; ok {
				siteID = fmt.Sprintf("%v", sid)
			}

			// Extract site_name: try city first, fall back to site_id
			if sn, ok := dataMap["city"]; ok {
				siteName = fmt.Sprintf("%v", sn)
			} else if sn, ok := dataMap["site_name"]; ok {
				siteName = fmt.Sprintf("%v", sn)
			} else {
				siteName = siteID
			}

			// Extract location: try "city, state", fall back to site_id
			if st, ok := dataMap["state"]; ok {
				if city, ok := dataMap["city"]; ok {
					location = fmt.Sprintf("%v, %v", city, st)
				} else {
					location = fmt.Sprintf("%v", st)
				}
			} else if loc, ok := dataMap["location"]; ok {
				location = fmt.Sprintf("%v", loc)
			} else {
				location = siteID
			}

			// Build type-coerced data: convert string values to proper
			// types based on schema field definitions
			coerced := make(map[string]interface{})
			for k, v := range dataMap {
				strVal, isStr := v.(string)
				if !isStr {
					coerced[k] = v
					continue
				}
				// Check if this field has a numeric type in the schema
				if fieldDef, exists := resolvedSchema.Fields[k]; exists {
					switch fieldDef.Type {
					case "percentage", "index", "numeric", "population":
						if f, err := strconv.ParseFloat(strVal, 64); err == nil {
							coerced[k] = f
							continue
						}
					case "integer":
						if f, err := strconv.ParseFloat(strVal, 64); err == nil {
							coerced[k] = int64(f)
							continue
						}
					}
				}
				coerced[k] = v // keep original string for text/identifier/unknown
			}
			coercedJSON, _ := json.Marshal(coerced)

			siteRecords[i] = models.SiteRecord{
				ID:        uuid.New(),
				UploadID:  uploadID,
				TenantID:  tenantID,
				SiteID:    siteID,
				SiteName:  siteName,
				Location:  location,
				RawData:   recordData,
				Data:      coercedJSON,
				CreatedAt: now,
			}
		} else {
			siteRecords[i] = models.SiteRecord{
				ID:        uuid.New(),
				UploadID:  uploadID,
				TenantID:  tenantID,
				SiteID:    siteID,
				RawData:   recordData,
				Data:      recordData,
				CreatedAt: now,
			}
		}
	}

	if err := h.siteRecordRepo.BulkInsert(c.Request.Context(), siteRecords); err != nil {
		os.Remove(tempPath)
		upload.ValidationStatus = "invalid"
		upload.UpdatedAt = time.Now()
		_ = h.uploadRepo.Update(c.Request.Context(), upload)
		response.InternalError(c, fmt.Sprintf("failed to insert site records: %v", err))
		return
	}

	// Update upload as valid
	upload.ValidationStatus = "valid"
	upload.Status = "completed"
	upload.RowCount = len(records)
	upload.SchemaVersion = "v1.0"
	upload.UpdatedAt = time.Now()

	if err := h.uploadRepo.Update(c.Request.Context(), upload); err != nil {
		response.InternalError(c, fmt.Sprintf("failed to update upload: %v", err))
		return
	}

	os.Remove(tempPath)

	// Build response matching case study spec (includes validation_warnings)
	uploadResponse := gin.H{
		"upload_id":           upload.ID,
		"tenant_id":           upload.TenantID,
		"filename":            upload.Filename,
		"row_count":           upload.RowCount,
		"schema_version":      upload.SchemaVersion,
		"validation_status":   upload.ValidationStatus,
		"validation_warnings": parseWarnings,
		"created_at":          upload.CreatedAt,
	}

	response.Success(c, http.StatusCreated, uploadResponse)
}
