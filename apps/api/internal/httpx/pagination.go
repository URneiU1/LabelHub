package httpx

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Page struct {
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}

type PageResponse struct {
	Data      any    `json:"data"`
	Page      Page   `json:"page"`
	RequestID string `json:"request_id"`
}

func PageOK(c *gin.Context, data any, page Page) {
	c.JSON(http.StatusOK, PageResponse{
		Data:      data,
		Page:      page,
		RequestID: RequestID(c),
	})
}

func CursorLimit(c *gin.Context) int {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}
