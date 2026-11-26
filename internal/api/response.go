package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func fail(c *gin.Context, status int, code, message string, details any) {
	c.AbortWithStatusJSON(status, apiError{Code: code, Message: message, Details: details})
}

func ok(c *gin.Context, v any) {
	c.JSON(http.StatusOK, v)
}

type pageResponse struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}
