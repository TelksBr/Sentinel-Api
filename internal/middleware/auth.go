package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"

	"api-v2/internal/models"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware implementa o middleware de autenticação
type AuthMiddleware struct {
	apiKey string
}

// NewAuthMiddleware cria uma nova instância do middleware de autenticação
func NewAuthMiddleware(apiKey string) *AuthMiddleware {
	return &AuthMiddleware{
		apiKey: apiKey,
	}
}

// Middleware implementa o middleware de autenticação
func (a *AuthMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Obter o token de autorização do header
		authToken := c.GetHeader("Authorization")
		if authToken == "" {
			c.JSON(http.StatusUnauthorized, models.NewErrorResponse("Token de autorização não fornecido"))
			c.Abort()
			return
		}

		// Verificar prefixo Bearer
		const bearerPrefix = "Bearer "
		if len(authToken) < len(bearerPrefix) || authToken[:len(bearerPrefix)] != bearerPrefix {
			c.JSON(http.StatusUnauthorized, models.NewErrorResponse("Token de autorização inválido"))
			c.Abort()
			return
		}

		// Extrair token sem o prefixo
		providedKey := authToken[len(bearerPrefix):]

		// Comparação constant-time — imune a timing attacks
		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(a.apiKey)) != 1 {
			c.JSON(http.StatusUnauthorized, models.NewErrorResponse("Token de autorização inválido"))
			c.Abort()
			return
		}

		// Continuar para o próximo handler
		c.Next()
	}
}

// GetAPIKeyFromEnv obtém a API key da variável de ambiente
func GetAPIKeyFromEnv() (string, error) {
	apiKey := os.Getenv("API_SENTINEL_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("variável de ambiente API_SENTINEL_KEY não definida")
	}
	return apiKey, nil
}
