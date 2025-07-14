package cloudformation

import "github.com/gin-gonic/gin"

func CloudFormationRoutes(router *gin.RouterGroup) {
	router.POST("/download-template", DownloadCloudFormationTemplate)
}