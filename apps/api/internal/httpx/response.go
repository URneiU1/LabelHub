package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error     ErrorBody `json:"error"`
	RequestID string    `json:"request_id"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type Response struct {
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Data:      data,
		RequestID: RequestID(c),
	})
}

func Error(c *gin.Context, status int, code string, message string) {
	ErrorWithDetails(c, status, code, message, nil)
}

func ErrorWithDetails(c *gin.Context, status int, code string, message string, details any) {
	c.JSON(status, ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
		RequestID: RequestID(c),
	})
}

func RequestID(c *gin.Context) string {
	if value, ok := c.Get("request_id"); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return ""
}
