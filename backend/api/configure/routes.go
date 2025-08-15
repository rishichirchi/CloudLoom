package configure

import "github.com/gin-gonic/gin"

func SetupConfigureRoutes(router *gin.RouterGroup) {
	router.POST("/setup-cloudtrail", SetupCloudTrailHandler)
}
