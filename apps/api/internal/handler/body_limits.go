package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"labelhub-api/internal/httpx"
)

const (
	maxTemplateSchemaBytes = int64(256 * 1024)
	maxAnswerJSONBytes     = int64(128 * 1024)

	maxTemplateFields      = 80
	maxTemplateOptions     = 50
	maxTemplateStringBytes = 8 * 1024
)

func bindLimitedJSON(c *gin.Context, dst any, maxBytes int64) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	if err := c.ShouldBindJSON(dst); err != nil {
		if isRequestTooLarge(err) {
			httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body is too large")
			return false
		}
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "request body is invalid")
		return false
	}
	return true
}

func isRequestTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr) || strings.Contains(err.Error(), "request body too large")
}
