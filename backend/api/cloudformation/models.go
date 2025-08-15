package cloudformation

type CloudFormationRequest struct {
	AccessTier string `json:"accessTier"`
}

type CloudFormationResponse struct {
	Template     string `json:"template"`
	ExternalID   string `json:"externalId"`
	AccessTier   string `json:"accessTier"`
	TemplateType string `json:"templateType"`
}



const (
	CloudLoomNotificationTier = "CloudLoomNotificationTier"
	CloudLoomAutoApplyFixTier = "CloudLoomAutoApplyFixTier"
	CloudLoomSuggestFixTier   = "CloudLoomSuggestFixTier"
)
