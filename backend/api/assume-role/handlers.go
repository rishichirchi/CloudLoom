package assumerole

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rishichirchi/cloudloom/common"
	"github.com/rishichirchi/cloudloom/services"
)

type ARNRequest struct{
	RoleARN string `json:"arnNumber"`
}

// SetupCloudTrailHandler handles the HTTP request for CloudTrail setup
func SetupCloudTrailHandler(c *gin.Context) {
	log.Println("Setting Role ARN...")
	var req ARNRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"success": false,
		})
		return
	}

	common.ARNNumber = req.RoleARN

	service := services.NewCloudTrailService()

	err := service.SetupCloudTrail(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"success": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "CloudTrail and Auto Apply Fix setup completed successfully",
		"success": true,
	})
}

// SendTestMessageHandler handles the HTTP request for sending a test message to SQS
func SendTestMessageHandler(c *gin.Context) {
	service := services.NewCloudTrailService()

	err := service.SendTestMessage(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"success": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Test message sent successfully",
		"success": true,
	})
}
