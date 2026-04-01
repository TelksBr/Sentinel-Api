package routes

import (
	"api-v2/internal/cron"
	"api-v2/internal/handlers"
	"api-v2/internal/middleware"
	"api-v2/internal/services"

	"github.com/gin-gonic/gin"
)

// SetupRoutes configura todas as rotas da API
func SetupRoutes(sshService *services.SSHService, v2rayService *services.V2RayService, monitorService *services.MonitorService, cronService *cron.CronjobService, authMiddleware *middleware.AuthMiddleware) *gin.Engine {
	// Configurar Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// Handlers
	sshHandlers := handlers.NewSSHHandlers(sshService, cronService)
	v2rayHandlers := handlers.NewV2RayHandlers(v2rayService, cronService)
	monitorHandlers := handlers.NewMonitorHandlers(monitorService)

	// Rota de health check
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "🟢 API running !"})
	})

	// Rotas públicas (sem autenticação)
	r.GET("/onlines", monitorHandlers.GetOnlineUsers)              // GET /onlines
	r.GET("/system/resources", monitorHandlers.GetSystemResources) // GET /system/resources

	// Aplicar middleware de autenticação para rotas protegidas
	authorized := r.Group("/")
	authorized.Use(authMiddleware.Middleware())
	{
		// Rotas SSH
		ssh := authorized.Group("/ssh_user")
		{
			ssh.POST("", sshHandlers.CreateUsers)                  // POST /ssh_user
			ssh.PUT("/:username", sshHandlers.UpdateUser)          // PUT /ssh_user/:username
			ssh.POST("/delete", sshHandlers.DeleteUsers)           // POST /ssh_user/delete
			ssh.POST("/delete_all", sshHandlers.DeleteAllUsers)    // POST /ssh_user/delete_all
			ssh.POST("/test", sshHandlers.CreateTestUser)          // POST /ssh_user/test
			ssh.PUT("/disable/:username", sshHandlers.DisableUser) // PUT /ssh_user/disable/:username
			ssh.PUT("/enable/:username", sshHandlers.EnableUser)   // PUT /ssh_user/enable/:username
		}

		// Rotas V2Ray
		v2ray := authorized.Group("/v2ray_user")
		{
			v2ray.POST("", v2rayHandlers.CreateUsers)               // POST /v2ray_user
			v2ray.PUT("/:uuid", v2rayHandlers.UpdateValidate)       // PUT /v2ray_user/:uuid
			v2ray.POST("/delete", v2rayHandlers.DeleteUsers)        // POST /v2ray_user/delete
			v2ray.POST("/delete_all", v2rayHandlers.DeleteAllUsers) // POST /v2ray_user/delete_all
			v2ray.PUT("/disable/:uuid", v2rayHandlers.DisableUser)  // PUT /v2ray_user/disable/:uuid
			v2ray.PUT("/enable/:uuid", v2rayHandlers.EnableUser)    // PUT /v2ray_user/enable/:uuid
		}

		// Rota de teste V2Ray
		authorized.POST("/v2ray/test", v2rayHandlers.CreateTestUser) // POST /v2ray/test

		// Rotas de Monitoramento protegidas
		authorized.GET("/users/online", monitorHandlers.GetDetailedOnlineUsers) // GET /users/online
	}

	return r
}
