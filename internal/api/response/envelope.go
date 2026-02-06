package response

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Envelope is the standard API response wrapper.
type Envelope struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data,omitempty"`
	Error  *ErrorBody  `json:"error,omitempty"`
	Meta   Meta        `json:"meta"`
}

// ErrorBody holds error details in the response.
type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// Meta holds response metadata.
type Meta struct {
	CorrelationID string `json:"correlation_id"`
	Timestamp     string `json:"timestamp"`
}

func newMeta(c *gin.Context) Meta {
	corrID, _ := c.Get("correlation_id")
	corrIDStr, ok := corrID.(string)
	if !ok {
		corrIDStr = uuid.New().String()
	}
	return Meta{
		CorrelationID: corrIDStr,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}
}

// Success sends a successful response.
func Success(c *gin.Context, statusCode int, data interface{}) {
	c.JSON(statusCode, Envelope{
		Status: "success",
		Data:   data,
		Meta:   newMeta(c),
	})
}

// Error sends an error response.
func Error(c *gin.Context, statusCode int, code, message string, details interface{}) {
	c.JSON(statusCode, Envelope{
		Status: "error",
		Error: &ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: newMeta(c),
	})
}

// BadRequest sends a 400 error.
func BadRequest(c *gin.Context, message string, details interface{}) {
	Error(c, http.StatusBadRequest, "VALIDATION_ERROR", message, details)
}

// NotFound sends a 404 error.
func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, "NOT_FOUND", message, nil)
}

// Conflict sends a 409 error.
func Conflict(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusConflict, Envelope{
		Status: "success",
		Data:   data,
		Error: &ErrorBody{
			Code:    "DUPLICATE",
			Message: message,
		},
		Meta: newMeta(c),
	})
}

// InternalError sends a 500 error.
func InternalError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", message, nil)
}

// Unauthorized sends a 401 error.
func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, "UNAUTHORIZED", message, nil)
}

// Forbidden sends a 403 error.
func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, "FORBIDDEN", message, nil)
}
