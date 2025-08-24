package infrastructure

import (
	"github.com/gin-gonic/gin"
)

// SetupInfrastructureRoutes sets up the infrastructure-related routes
func SetupInfrastructureRoutes(router *gin.RouterGroup) {
	router.POST("/get-live-infrastructure-data", GetLiveInfrastructureData)
	router.POST("/generate-infrastructure-diagram", GenerateInfrastructureDiagram)
	router.GET("/get-mermaid-diagram-code", GetMermaidDiagramCode)
}
