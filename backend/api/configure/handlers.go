package configure

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rishichirchi/cloudloom/common"
	"github.com/rishichirchi/cloudloom/services"
)

type RoleARNRequest struct {
	ARNNumber      string `json:"arnNumber"`
	ExternalID     *string `json:"externalId"`
	GithubRepoLink *string `json:"githubRepoLink"`
}

// SetupCloudTrailHandler handles the HTTP request for CloudTrail setup
func SetupCloudTrailHandler(c *gin.Context) {
	var request RoleARNRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	common.ARNNumber = request.ARNNumber

	arn := fmt.Sprintf("ARN number: %s\nExternal ID: %s", common.ARNNumber, common.ExternalID)
	fmt.Printf("Received ARN request: %s\n", arn)

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
