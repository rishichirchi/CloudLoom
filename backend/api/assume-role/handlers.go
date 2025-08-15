package assumerole

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetupCloudTrailHandler handles the HTTP request for CloudTrail setup
func SetupCloudTrailHandler(c *gin.Context) {
	service := NewCloudTrailService()

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
	service := NewCloudTrailService()

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
