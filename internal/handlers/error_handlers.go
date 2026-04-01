package handlers

import (
	"net/http"

	"api-v2/internal/models"
	"github.com/gin-gonic/gin"
)

// HandleBadRequest retorna uma resposta de erro 400
func HandleBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, models.NewErrorResponse(message))
}

// HandleError retorna uma resposta de erro 500
func HandleError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, models.NewErrorResponse(err.Error()))
}

// HandleNotFound retorna uma resposta de erro 404
func HandleNotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, models.NewErrorResponse(message))
}

// HandleValidationError retorna uma resposta de erro de validação
func HandleValidationError(c *gin.Context, message string, details []models.ValidationError) {
	c.JSON(http.StatusBadRequest, models.NewValidationErrorResponse(message, details))
}

// HandleUnauthorized retorna uma resposta de erro 401
func HandleUnauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, models.NewErrorResponse(message))
}
