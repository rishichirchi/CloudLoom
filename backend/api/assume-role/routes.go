package assumerole

import "github.com/gin-gonic/gin"

func SetupAssumeRoleRoutes(router *gin.RouterGroup) {
	router.POST("/setup-cloudtrail", SetupCloudTrailHandler)
	router.POST("/test-sqs", SendTestMessageHandler)
}
