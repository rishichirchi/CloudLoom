package route

import (
	"github.com/gin-gonic/gin"
	"github.com/rishichirchi/cloudloom/api/cloudformation"
	"github.com/rishichirchi/cloudloom/api/configure"
	"github.com/rishichirchi/cloudloom/api/infrastructure"
)

func SetupRoutes(router *gin.Engine) {
	v1 := router.Group("/api/v1")

	// Health check route
	v1.GET("/", func(c *gin.Context) {
		c.String(200, "Hello, World!")
	})

	cloudFormationRouterGroup := v1.Group("/cloudformation")
	cloudformation.CloudFormationRoutes(cloudFormationRouterGroup)

	assumeRoleRouterGroup := v1.Group("/configure")
	configure.SetupConfigureRoutes(assumeRoleRouterGroup)

	infrastructureRouterGroup := v1.Group("/infrastructure")
	infrastructure.SetupInfrastructureRoutes(infrastructureRouterGroup)
}
