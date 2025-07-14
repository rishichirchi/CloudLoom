package route

import (
	"github.com/gin-gonic/gin"
	"github.com/rishichirchi/cloudloom/api/cloudformation"
)

func SetupRoutes(router *gin.Engine) {
	v1 := router.Group("/api/v1")

	// Health check route
	v1.GET("/", func(c *gin.Context) {
		c.String(200, "Hello, World!")
	})

	cloudFormationRouterGroup := v1.Group("/cloudformation")
	cloudformation.CloudFormationRoutes(cloudFormationRouterGroup)
}