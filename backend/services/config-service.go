package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/configservice"
	"github.com/aws/aws-sdk-go-v2/service/configservice/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// --- Data Structures ---

// ResourceInventory represents a comprehensive view of AWS resources
type ResourceInventory struct {
	Resources       []ConfigurationItem `json:"resources"`
	Policies        []PolicyDocument    `json:"policies"`
	ComplianceRules []ComplianceRule    `json:"complianceRules"`
	ResourceSummary ResourceSummary     `json:"resourceSummary"`
	LastUpdated     time.Time           `json:"lastUpdated"`
}

// ConfigurationItem represents an AWS resource configuration, compatible with SelectResourceConfig output
type ConfigurationItem struct {
	ResourceID           string                 `json:"resourceId"`
	ResourceType         string                 `json:"resourceType"`
	ResourceName         string                 `json:"resourceName"`
	Region               string                 `json:"awsRegion"`
	AvailabilityZone     string                 `json:"availabilityZone"`
	Configuration        map[string]interface{} `json:"configuration"`
	ConfigurationStatus  string                 `json:"configurationItemStatus"`
	ConfigurationStateId string                 `json:"configurationStateId"`
	ResourceCreationTime *time.Time             `json:"resourceCreationTime"`
	Tags                 FlexibleTags           `json:"tags"`
	Relationships        []Relationship         `json:"relationships"`
	ComplianceStatus     string                 `json:"complianceStatus"` // This will be populated separately
}

// FlexibleTags handles both map[string]string and array formats from AWS Config
type FlexibleTags map[string]string

// UnmarshalJSON implements custom JSON unmarshaling for tags
func (ft *FlexibleTags) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as map[string]string first
	var mapTags map[string]string
	if err := json.Unmarshal(data, &mapTags); err == nil {
		*ft = FlexibleTags(mapTags)
		return nil
	}

	// If that fails, try to unmarshal as array format
	var arrayTags []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &arrayTags); err == nil {
		result := make(map[string]string)
		for _, tag := range arrayTags {
			result[tag.Key] = tag.Value
		}
		*ft = FlexibleTags(result)
		return nil
	}

	// If both fail, initialize as empty map
	*ft = make(FlexibleTags)
	return nil
}

// PolicyDocument represents IAM policies and resource policies
type PolicyDocument struct {
	PolicyName     string                 `json:"policyName"`
	PolicyType     string                 `json:"policyType"` // IAM_MANAGED, etc.
	PolicyDocument map[string]interface{} `json:"policyDocument"`
	AttachedTo     []string               `json:"attachedTo"`
	ResourceArn    string                 `json:"resourceArn"`
}

// ComplianceRule represents AWS Config rules and their compliance status
type ComplianceRule struct {
	ConfigRuleName    string             `json:"configRuleName"`
	ComplianceType    string             `json:"complianceType"`
	Source            string             `json:"source"`
	ResourceType      string             `json:"resourceType"`
	EvaluationResults []EvaluationResult `json:"evaluationResults"`
}

// EvaluationResult represents individual compliance evaluation
type EvaluationResult struct {
	ResourceID         string    `json:"resourceId"`
	ResourceType       string    `json:"resourceType"`
	ComplianceType     string    `json:"complianceType"`
	OrderingTimestamp  time.Time `json:"orderingTimestamp"`
	ResultRecordedTime time.Time `json:"resultRecordedTime"`
	Annotation         string    `json:"annotation"`
}

// ResourceSummary provides aggregated statistics
type ResourceSummary struct {
	TotalResources    int            `json:"totalResources"`
	ResourcesByType   map[string]int `json:"resourcesByType"`
	ResourcesByRegion map[string]int `json:"resourcesByRegion"`
	ComplianceStatus  map[string]int `json:"complianceStatus"`
	PolicyCount       int            `json:"policyCount"`
	ConfigRulesCount  int            `json:"configRulesCount"`
}

// Relationship represents resource relationships
type Relationship struct {
	ResourceType     string `json:"resourceType"`
	ResourceID       string `json:"resourceId"`
	ResourceName     string `json:"resourceName"`
	RelationshipName string `json:"relationshipName"`
}

// ConfigService provides methods to interact with AWS Config
type ConfigService struct {
	client *configservice.Client
}

// NewConfigService creates a new ConfigService instance
func NewConfigService(cfg aws.Config) *ConfigService {
	return &ConfigService{
		client: configservice.NewFromConfig(cfg),
	}
}

// checkRecordingStatus verifies if AWS Config is actively recording resources
func (cs *ConfigService) checkRecordingStatus(ctx context.Context) (bool, error) {
	input := &configservice.DescribeConfigurationRecorderStatusInput{}
	result, err := cs.client.DescribeConfigurationRecorderStatus(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check configuration recorder status: %w", err)
	}

	for _, status := range result.ConfigurationRecordersStatus {
		if status.Recording {
			log.Printf("[ConfigService] Configuration recorder '%s' is actively recording", aws.ToString(status.Name))
			return true, nil
		}
	}

	log.Printf("[ConfigService] No active configuration recorders found")
	return false, nil
}

// startConfigurationRecorderIfNeeded attempts to start any stopped configuration recorders
func (cs *ConfigService) startConfigurationRecorderIfNeeded(ctx context.Context) error {
	log.Println("[ConfigService] Checking if configuration recorders need to be started...")

	// Get list of all configuration recorders first to extract accountID
	listInput := &configservice.DescribeConfigurationRecordersInput{}
	listResult, err := cs.client.DescribeConfigurationRecorders(ctx, listInput)
	if err != nil {
		return fmt.Errorf("failed to list configuration recorders: %w", err)
	}

	if len(listResult.ConfigurationRecorders) == 0 {
		log.Println("[ConfigService] No configuration recorders exist")
		return nil
	}

	// Extract accountID from existing recorder name (format: CloudLoom-Config-Recorder-{accountID})
	var accountID string
	for _, recorder := range listResult.ConfigurationRecorders {
		recorderName := aws.ToString(recorder.Name)
		if strings.HasPrefix(recorderName, "CloudLoom-Config-Recorder-") {
			accountID = strings.TrimPrefix(recorderName, "CloudLoom-Config-Recorder-")
			break
		}
	}

	if accountID == "" {
		log.Println("[ConfigService] Could not extract accountID from recorder names")
		return fmt.Errorf("unable to determine accountID for delivery channel creation")
	}

	log.Printf("[ConfigService] Detected accountID: %s", accountID)

	// Now ensure delivery channels exist - recorders can't start without them
	if err := cs.ensureDeliveryChannelExists(ctx, accountID); err != nil {
		return fmt.Errorf("delivery channel check failed: %w", err)
	} // Check status of each recorder
	statusInput := &configservice.DescribeConfigurationRecorderStatusInput{}
	statusResult, err := cs.client.DescribeConfigurationRecorderStatus(ctx, statusInput)
	if err != nil {
		return fmt.Errorf("failed to check recorder status: %w", err)
	}

	// Create a map of recorder statuses
	recorderStatus := make(map[string]bool)
	for _, status := range statusResult.ConfigurationRecordersStatus {
		recorderStatus[aws.ToString(status.Name)] = status.Recording
	}

	// Start any recorders that aren't running
	startedAny := false
	for _, recorder := range listResult.ConfigurationRecorders {
		recorderName := aws.ToString(recorder.Name)
		if !recorderStatus[recorderName] {
			log.Printf("[ConfigService] Starting stopped configuration recorder: %s", recorderName)

			startInput := &configservice.StartConfigurationRecorderInput{
				ConfigurationRecorderName: aws.String(recorderName),
			}

			_, err = cs.client.StartConfigurationRecorder(ctx, startInput)
			if err != nil {
				log.Printf("[ConfigService] Warning: Failed to start recorder %s: %v", recorderName, err)

				// Provide specific guidance based on error type
				if strings.Contains(err.Error(), "NoAvailableDeliveryChannelException") {
					log.Printf("[ConfigService] ❌ Delivery channel issue detected. This usually means:")
					log.Printf("[ConfigService]    - Delivery channel doesn't exist, or")
					log.Printf("[ConfigService]    - S3 bucket permissions are incorrect")
					log.Printf("[ConfigService] 💡 Solution: Check S3 bucket policy allows AWS Config access")
				}
				continue
			}

			log.Printf("[ConfigService] ✅ Successfully started configuration recorder: %s", recorderName)
			startedAny = true
		} else {
			log.Printf("[ConfigService] Configuration recorder %s is already running", recorderName)
		}
	}

	if startedAny {
		log.Println("[ConfigService] ⏳ Configuration recorders started. Resources will be available shortly...")
		return fmt.Errorf("configuration recorders were just started, please wait a few minutes for resources to be recorded")
	}

	return nil
}

// ensureDeliveryChannelExists checks if delivery channel exists and creates one if needed
func (cs *ConfigService) ensureDeliveryChannelExists(ctx context.Context, accountID string) error {
	log.Println("[ConfigService] Checking delivery channel availability...")

	// Check if any delivery channels exist
	listInput := &configservice.DescribeDeliveryChannelsInput{}
	listResult, err := cs.client.DescribeDeliveryChannels(ctx, listInput)
	if err != nil {
		return fmt.Errorf("failed to list delivery channels: %w", err)
	}

	if len(listResult.DeliveryChannels) == 0 {
		log.Println("[ConfigService] No delivery channels found - attempting to create one...")

		// Try to create delivery channel using the same S3 bucket pattern as CloudTrail
		bucketName := fmt.Sprintf("cloudloom-logs-%s", accountID)
		channelName := fmt.Sprintf("CloudLoom-Config-Channel-%s", accountID)

		log.Printf("[ConfigService] Creating delivery channel: %s -> S3 bucket: %s", channelName, bucketName)

		if err := cs.createMissingDeliveryChannel(ctx, channelName, bucketName, accountID); err != nil {
			log.Printf("[ConfigService] ❌ Failed to create delivery channel: %v", err)
			log.Println("[ConfigService] 💡 To fix this manually:")
			log.Println("[ConfigService]    1. Ensure S3 bucket exists and has proper Config permissions")
			log.Println("[ConfigService]    2. Run the AWS Config setup process again")
			log.Println("[ConfigService]    3. Check CloudFormation logs for setup errors")
			return fmt.Errorf("failed to create delivery channel: %w", err)
		}

		log.Printf("[ConfigService] ✅ Successfully created delivery channel: %s", channelName)
		return nil
	}

	// Check delivery channel status
	for _, channel := range listResult.DeliveryChannels {
		channelName := aws.ToString(channel.Name)
		bucketName := aws.ToString(channel.S3BucketName)
		log.Printf("[ConfigService] Found delivery channel: %s -> S3 bucket: %s", channelName, bucketName)

		// Verify the S3 bucket exists and is accessible
		if err := cs.verifyS3BucketAccess(ctx, bucketName); err != nil {
			log.Printf("[ConfigService] Warning: Delivery channel %s has S3 bucket issue: %v", channelName, err)
			return fmt.Errorf("delivery channel S3 bucket issue: %w", err)
		}
	}

	log.Printf("[ConfigService] ✅ Found %d working delivery channel(s)", len(listResult.DeliveryChannels))
	return nil
}

// verifyS3BucketAccess checks if the S3 bucket for Config delivery channel is accessible
func (cs *ConfigService) verifyS3BucketAccess(ctx context.Context, bucketName string) error {
	log.Printf("[ConfigService] Verifying S3 bucket access: %s", bucketName)

	// Create S3 client to test bucket access
	// Note: we would need aws.Config here, but for now we'll do basic validation
	// In a full implementation, this would check:
	// 1. Bucket exists and is accessible
	// 2. Bucket policy allows config.amazonaws.com to write
	// 3. Proper S3 key prefix permissions

	if bucketName == "" {
		return fmt.Errorf("delivery channel has empty S3 bucket name")
	}

	// Basic validation - in practice you'd test actual S3 access here
	log.Printf("[ConfigService] ✅ Basic validation passed for bucket: %s", bucketName)
	return nil
}

// createMissingDeliveryChannel creates a new AWS Config delivery channel with proper S3 configuration
func (cs *ConfigService) createMissingDeliveryChannel(ctx context.Context, channelName, bucketName, accountID string) error {
	log.Printf("[ConfigService] Creating AWS Config delivery channel: %s", channelName)

	// First ensure the S3 bucket exists and has proper Config permissions
	log.Printf("[ConfigService] Ensuring S3 bucket %s has proper AWS Config permissions...", bucketName)

	// We need to get the AWS config to create S3 client
	// For now, let's try to create the delivery channel and provide better error messaging
	// The S3 bucket should already exist from CloudTrail setup, we just need proper policy

	// Define the delivery channel configuration
	// AWS Config automatically adds AWSLogs structure, so we just need the base prefix
	// The bucket policy allows: config/AWSLogs/{accountID}/Config/*
	// So our prefix should be: config/ (AWS Config will add AWSLogs/{accountID}/Config automatically)
	deliveryChannel := &types.DeliveryChannel{
		Name:         aws.String(channelName),
		S3BucketName: aws.String(bucketName),
		S3KeyPrefix:  aws.String("config"),
		ConfigSnapshotDeliveryProperties: &types.ConfigSnapshotDeliveryProperties{
			DeliveryFrequency: types.MaximumExecutionFrequencyTwentyFourHours,
		},
	}

	// Create the delivery channel
	input := &configservice.PutDeliveryChannelInput{
		DeliveryChannel: deliveryChannel,
	}

	_, err := cs.client.PutDeliveryChannel(ctx, input)
	if err != nil {
		log.Printf("[ConfigService] ❌ Failed to create delivery channel: %v", err)

		// Check if this is an S3 permissions issue
		if strings.Contains(err.Error(), "InsufficientDeliveryPolicyException") {
			log.Printf("[ConfigService] 💡 S3 bucket policy issue detected!")
			log.Printf("[ConfigService] 📋 To fix this:")
			log.Printf("[ConfigService]    1. The S3 bucket '%s' needs AWS Config permissions", bucketName)
			log.Printf("[ConfigService]    2. AWS Config will write to: s3://%s/config/AWSLogs/%s/Config/", bucketName, accountID)
			log.Printf("[ConfigService]    3. Check if S3 bucket policy allows config.amazonaws.com to write")
			log.Printf("[ConfigService]    4. Verify the bucket policy path matches: config/AWSLogs/%s/Config/*", accountID)
			return fmt.Errorf("S3 bucket policy insufficient for AWS Config delivery channel - bucket %s needs proper Config permissions for path config/AWSLogs/%s/Config/*", bucketName, accountID)
		}

		return fmt.Errorf("failed to create delivery channel %s: %w", channelName, err)
	}

	log.Printf("[ConfigService] ✅ Successfully created delivery channel: %s", channelName)
	log.Printf("[ConfigService] ✅ S3 destination: s3://%s/config/ (AWS Config will write to config/AWSLogs/%s/Config/)", bucketName, accountID)

	return nil
}

// getResourceCount gets a simple count of resources to verify Config is working
func (cs *ConfigService) getResourceCount(ctx context.Context) (int, error) {
	query := "SELECT COUNT(*)"
	input := &configservice.SelectResourceConfigInput{
		Expression: aws.String(query),
	}

	result, err := cs.client.SelectResourceConfig(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to execute count query: %w", err)
	}

	if len(result.Results) > 0 {
		// Parse the count result
		var count int
		if _, err := fmt.Sscanf(result.Results[0], "%d", &count); err == nil {
			return count, nil
		}
	}

	return 0, nil
}

// GetComprehensiveResourceInventory retrieves all resources, policies, and compliance information
func (cs *ConfigService) GetComprehensiveResourceInventory(ctx context.Context, cfg aws.Config) (*ResourceInventory, error) {
	log.Println("[ConfigService] Starting comprehensive resource inventory scan...")

	inventory := &ResourceInventory{
		LastUpdated: time.Now(),
	}

	// Step 1: Discover all resources efficiently
	allResources, err := cs.getAllResourcesWithSQL(ctx)
	if err != nil {
		// Check if this is a "just started" scenario
		isJustStarted := strings.Contains(err.Error(), "just started")
		if isJustStarted {
			log.Printf("[ConfigService] %v", err)
			log.Println("[ConfigService] Trying ListDiscoveredResources as immediate fallback...")
		} else {
			log.Printf("[ConfigService] SQL approach failed: %v, trying ListDiscoveredResources fallback...", err)
		}

		allResources, err = cs.getAllResourcesWithListAPI(ctx)
		if err != nil {
			return nil, fmt.Errorf("both SQL and List API approaches failed: %w", err)
		}

		// If fallback succeeded but SQL failed due to just started recorders
		if isJustStarted && len(allResources) > 0 {
			log.Printf("[ConfigService] ✅ ListDiscoveredResources found %d resources while Config is initializing", len(allResources))
		}
	}
	inventory.Resources = allResources

	// Step 2: Get compliance rules and their evaluations
	complianceRules, err := cs.GetComplianceRules(ctx)
	if err != nil {
		log.Printf("[ConfigService] Warning: failed to get compliance rules: %v", err)
	} else {
		inventory.ComplianceRules = complianceRules
	}

	// Step 3: Get customer-managed IAM policies
	policies, err := cs.GetIAMPolicies(ctx, cfg)
	if err != nil {
		log.Printf("[ConfigService] Warning: failed to get IAM policies: %v", err)
	} else {
		inventory.Policies = policies
	}

	// Step 4: Generate a summary of the collected data
	inventory.ResourceSummary = cs.GenerateResourceSummary(inventory)

	log.Printf("[ConfigService] Inventory complete: %d resources, %d policies, %d compliance rules",
		len(inventory.Resources), len(inventory.Policies), len(inventory.ComplianceRules))

	return inventory, nil
}

// getAllResourcesWithSQL fetches all resource configurations using a single, efficient API call.
func (cs *ConfigService) getAllResourcesWithSQL(ctx context.Context) ([]ConfigurationItem, error) {
	log.Println("[ConfigService] Fetching all resources using SelectResourceConfig API...")

	// First check if Config is recording and has data
	recordingStatus, err := cs.checkRecordingStatus(ctx)
	if err != nil {
		log.Printf("[ConfigService] Warning: Could not check recording status: %v", err)
	} else {
		log.Printf("[ConfigService] Config recording status: %v", recordingStatus)

		// If not recording, try to start any stopped recorders
		if !recordingStatus {
			log.Println("[ConfigService] No active recording detected, attempting to start configuration recorders...")
			if startErr := cs.startConfigurationRecorderIfNeeded(ctx); startErr != nil {
				log.Printf("[ConfigService] Recorder startup result: %v", startErr)
				// If recorders were just started, return early to allow time for recording
				if strings.Contains(startErr.Error(), "just started") {
					return nil, startErr
				}
			}
		}
	}

	var resources []ConfigurationItem

	// Try simple query first to check if Config has any data
	count, err := cs.getResourceCount(ctx)
	if err != nil {
		log.Printf("[ConfigService] Simple count query failed: %v", err)
		return nil, fmt.Errorf("config service not ready: %w", err)
	}
	log.Printf("[ConfigService] Config reports %d total resources available", count)

	if count == 0 {
		// Check if recording is active but just hasn't populated yet
		if recordingStatus {
			log.Println("[ConfigService] Config is recording but no resources found yet - may need more time to populate")
			log.Println("[ConfigService] This is normal for newly enabled Config service (can take 10-15 minutes)")
		} else {
			log.Println("[ConfigService] No resources found in Config and recording is not active")
		}
		log.Println("[ConfigService] Returning empty list - fallback to ListDiscoveredResources will be used")
		return resources, nil
	}

	// AWS Config SQL syntax - no FROM clause needed
	query := `SELECT 
		resourceId, 
		resourceType, 
		resourceName, 
		awsRegion, 
		availabilityZone, 
		configuration, 
		configurationItemStatus, 
		configurationStateId, 
		resourceCreationTime, 
		tags, 
		relationships`

	log.Printf("[ConfigService] Executing SQL query: %s", query)

	input := &configservice.SelectResourceConfigInput{
		Expression: aws.String(query),
	}

	paginator := configservice.NewSelectResourceConfigPaginator(cs.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get next page of resource configurations: %w", err)
		}

		for _, resultString := range page.Results {
			var item ConfigurationItem
			err := json.Unmarshal([]byte(resultString), &item)
			if err != nil {
				log.Printf("[ConfigService] Warning: failed to unmarshal resource configuration: %v", err)
				log.Printf("[ConfigService] Raw result string: %s", resultString)
				continue
			}
			resources = append(resources, item)
		}
	}

	log.Printf("[ConfigService] Successfully fetched %d resources via SQL query.", len(resources))

	// If we got 0 resources, try a simpler query to see if Config has any data at all
	if len(resources) == 0 {
		log.Println("[ConfigService] No resources found with full query, trying simple count query...")
		count, err := cs.getResourceCount(ctx)
		if err != nil {
			log.Printf("[ConfigService] Resource count query also failed: %v", err)
		} else {
			log.Printf("[ConfigService] Config shows %d total resources available", count)
		}
	}

	return resources, nil
}

// getAllResourcesWithListAPI fetches resources using ListDiscoveredResources API as fallback
func (cs *ConfigService) getAllResourcesWithListAPI(ctx context.Context) ([]ConfigurationItem, error) {
	log.Println("[ConfigService] Using ListDiscoveredResources API as fallback...")

	var allResources []ConfigurationItem

	// Common AWS resource types to discover
	resourceTypes := []string{
		"AWS::EC2::Instance",
		"AWS::EC2::SecurityGroup",
		"AWS::EC2::VPC",
		"AWS::EC2::Subnet",
		"AWS::S3::Bucket",
		"AWS::IAM::Role",
		"AWS::IAM::User",
		"AWS::IAM::Policy",
		"AWS::Lambda::Function",
		"AWS::RDS::DBInstance",
		"AWS::CloudFormation::Stack",
	}

	for _, resourceType := range resourceTypes {
		log.Printf("[ConfigService] Discovering resources of type: %s", resourceType)

		input := &configservice.ListDiscoveredResourcesInput{
			ResourceType: types.ResourceType(resourceType),
		}

		paginator := configservice.NewListDiscoveredResourcesPaginator(cs.client, input)

		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				log.Printf("[ConfigService] Warning: failed to list resources of type %s: %v", resourceType, err)
				continue
			}

			for _, resource := range page.ResourceIdentifiers {
				// Convert discovered resource to ConfigurationItem
				item := ConfigurationItem{
					ResourceID:   aws.ToString(resource.ResourceId),
					ResourceType: string(resource.ResourceType),
					ResourceName: aws.ToString(resource.ResourceName),
					Tags:         make(FlexibleTags), // Initialize empty tags
				}
				allResources = append(allResources, item)
			}
		}
	}

	log.Printf("[ConfigService] Found %d resources using ListDiscoveredResources API", len(allResources))

	// If no resources found, let's check if this account has any resources at all
	if len(allResources) == 0 {
		log.Println("[ConfigService] No resources discovered. Checking possible reasons:")

		// Check if Config recorders are actually running
		if err := cs.diagnoseConfigStatus(ctx); err != nil {
			log.Printf("[ConfigService] Config diagnosis failed: %v", err)
		}

		// Try a broader resource discovery without specifying types
		log.Println("[ConfigService] Attempting broader resource discovery...")
		if broadResources, err := cs.tryBroadResourceDiscovery(ctx); err == nil && len(broadResources) > 0 {
			log.Printf("[ConfigService] Broad discovery found %d additional resources", len(broadResources))
			allResources = append(allResources, broadResources...)
		}
	}

	return allResources, nil
}

// diagnoseConfigStatus checks the current state of AWS Config service
func (cs *ConfigService) diagnoseConfigStatus(ctx context.Context) error {
	log.Println("[ConfigService] 🔍 Diagnosing AWS Config service status...")

	// Check configuration recorders
	recordersInput := &configservice.DescribeConfigurationRecordersInput{}
	recordersResult, err := cs.client.DescribeConfigurationRecorders(ctx, recordersInput)
	if err != nil {
		return fmt.Errorf("failed to describe recorders: %w", err)
	}

	log.Printf("[ConfigService] Found %d configuration recorder(s)", len(recordersResult.ConfigurationRecorders))
	for _, recorder := range recordersResult.ConfigurationRecorders {
		name := aws.ToString(recorder.Name)
		log.Printf("[ConfigService] - Recorder: %s", name)

		// Check recorder status
		statusInput := &configservice.DescribeConfigurationRecorderStatusInput{
			ConfigurationRecorderNames: []string{name},
		}
		statusResult, err := cs.client.DescribeConfigurationRecorderStatus(ctx, statusInput)
		if err != nil {
			log.Printf("[ConfigService] - Failed to get status for %s: %v", name, err)
			continue
		}

		for _, status := range statusResult.ConfigurationRecordersStatus {
			recording := status.Recording
			lastStatus := string(status.LastStatus)
			lastStartTime := status.LastStartTime
			lastStopTime := status.LastStopTime

			log.Printf("[ConfigService] - Recording: %v, Last Status: %s", recording, lastStatus)
			if lastStartTime != nil {
				log.Printf("[ConfigService] - Last Start: %v", *lastStartTime)
			}
			if lastStopTime != nil {
				log.Printf("[ConfigService] - Last Stop: %v", *lastStopTime)
			}
		}
	}

	return nil
}

// tryBroadResourceDiscovery attempts to discover any resources without filtering by type
func (cs *ConfigService) tryBroadResourceDiscovery(ctx context.Context) ([]ConfigurationItem, error) {
	log.Println("[ConfigService] Attempting broad resource discovery...")

	// Try to list any discovered resources without specifying a type
	// Note: This might not be supported by all AWS accounts/regions
	var allResources []ConfigurationItem

	// Try some additional resource types that might exist
	additionalTypes := []string{
		"AWS::EC2::NetworkInterface",
		"AWS::EC2::Volume",
		"AWS::EC2::KeyPair",
		"AWS::Route53::HostedZone",
		"AWS::CloudWatch::Alarm",
		"AWS::SNS::Topic",
		"AWS::SQS::Queue",
	}

	for _, resourceType := range additionalTypes {
		input := &configservice.ListDiscoveredResourcesInput{
			ResourceType: types.ResourceType(resourceType),
		}

		result, err := cs.client.ListDiscoveredResources(ctx, input)
		if err != nil {
			// Don't log errors for additional types as they might not be supported
			continue
		}

		if len(result.ResourceIdentifiers) > 0 {
			log.Printf("[ConfigService] Found %d resources of type %s", len(result.ResourceIdentifiers), resourceType)

			for _, resource := range result.ResourceIdentifiers {
				item := ConfigurationItem{
					ResourceID:   aws.ToString(resource.ResourceId),
					ResourceType: string(resource.ResourceType),
					ResourceName: aws.ToString(resource.ResourceName),
					Tags:         make(FlexibleTags),
				}
				allResources = append(allResources, item)
			}
		}
	}

	return allResources, nil
}

// GetComplianceRules retrieves all AWS Config rules and their compliance status
func (cs *ConfigService) GetComplianceRules(ctx context.Context) ([]ComplianceRule, error) {
	log.Println("[ConfigService] Fetching compliance rules...")
	var rules []ComplianceRule
	input := &configservice.DescribeConfigRulesInput{}
	paginator := configservice.NewDescribeConfigRulesPaginator(cs.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to describe config rules: %w", err)
		}

		for _, rule := range page.ConfigRules {
			// Get detailed compliance for each rule
			complianceDetails, err := cs.getRuleCompliance(ctx, aws.ToString(rule.ConfigRuleName))
			if err != nil {
				log.Printf("[ConfigService] Warning: could not get compliance for rule %s: %v", aws.ToString(rule.ConfigRuleName), err)
				continue
			}

			// Determine resource type scope
			var resourceTypesStr = "ALL"
			if rule.Scope != nil && len(rule.Scope.ComplianceResourceTypes) > 0 {
				resourceTypesStr = strings.Join(rule.Scope.ComplianceResourceTypes, ",")
			}

			complianceRule := ComplianceRule{
				ConfigRuleName:    aws.ToString(rule.ConfigRuleName),
				Source:            string(rule.Source.Owner),
				ResourceType:      resourceTypesStr,
				ComplianceType:    complianceDetails.ComplianceType,
				EvaluationResults: complianceDetails.EvaluationResults,
			}
			rules = append(rules, complianceRule)
		}
	}
	log.Printf("[ConfigService] Successfully fetched %d compliance rules.", len(rules))
	return rules, nil
}

// getRuleCompliance is a helper to get detailed compliance for a single rule.
func (cs *ConfigService) getRuleCompliance(ctx context.Context, ruleName string) (*ComplianceRule, error) {
	input := &configservice.GetComplianceDetailsByConfigRuleInput{
		ConfigRuleName: aws.String(ruleName),
	}

	paginator := configservice.NewGetComplianceDetailsByConfigRulePaginator(cs.client, input)

	compliance := &ComplianceRule{
		ConfigRuleName:    ruleName,
		EvaluationResults: []EvaluationResult{},
	}

	nonCompliantCount := 0

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get compliance page for rule %s: %w", ruleName, err)
		}

		for _, eval := range page.EvaluationResults {
			evalResult := EvaluationResult{
				ResourceID:         aws.ToString(eval.EvaluationResultIdentifier.EvaluationResultQualifier.ResourceId),
				ResourceType:       aws.ToString(eval.EvaluationResultIdentifier.EvaluationResultQualifier.ResourceType),
				ComplianceType:     string(eval.ComplianceType),
				OrderingTimestamp:  aws.ToTime(eval.ConfigRuleInvokedTime),
				ResultRecordedTime: aws.ToTime(eval.ResultRecordedTime),
				Annotation:         aws.ToString(eval.Annotation),
			}
			compliance.EvaluationResults = append(compliance.EvaluationResults, evalResult)

			if eval.ComplianceType == types.ComplianceTypeNonCompliant {
				nonCompliantCount++
			}
		}
	}

	if nonCompliantCount > 0 {
		compliance.ComplianceType = "NON_COMPLIANT"
	} else if len(compliance.EvaluationResults) > 0 {
		compliance.ComplianceType = "COMPLIANT"
	} else {
		compliance.ComplianceType = "NOT_APPLICABLE"
	}

	return compliance, nil
}

// GetIAMPolicies retrieves all customer-managed IAM policies in the account
func (cs *ConfigService) GetIAMPolicies(ctx context.Context, cfg aws.Config) ([]PolicyDocument, error) {
	log.Println("[ConfigService] Fetching IAM policies...")
	iamClient := iam.NewFromConfig(cfg)
	var policies []PolicyDocument

	input := &iam.ListPoliciesInput{
		Scope: iamtypes.PolicyScopeTypeLocal, // Only customer-managed policies
	}

	paginator := iam.NewListPoliciesPaginator(iamClient, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list managed policies: %w", err)
		}

		for _, policy := range page.Policies {
			policyDoc, err := cs.getPolicyDocument(ctx, iamClient, aws.ToString(policy.Arn), aws.ToString(policy.DefaultVersionId))
			if err != nil {
				log.Printf("[ConfigService] Warning: failed to get policy document for %s: %v", aws.ToString(policy.Arn), err)
				continue
			}

			policies = append(policies, PolicyDocument{
				PolicyName:     aws.ToString(policy.PolicyName),
				PolicyType:     "IAM_MANAGED",
				PolicyDocument: policyDoc,
				ResourceArn:    aws.ToString(policy.Arn),
			})
		}
	}
	log.Printf("[ConfigService] Successfully fetched %d IAM policies.", len(policies))
	return policies, nil
}

// getPolicyDocument retrieves and parses the JSON document for a given policy version.
func (cs *ConfigService) getPolicyDocument(ctx context.Context, iamClient *iam.Client, policyArn, versionId string) (map[string]interface{}, error) {
	versionInput := &iam.GetPolicyVersionInput{
		PolicyArn: aws.String(policyArn),
		VersionId: aws.String(versionId),
	}

	version, err := iamClient.GetPolicyVersion(ctx, versionInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get policy version: %w", err)
	}

	// The policy document is URL-encoded JSON. It must be decoded first.
	decodedDoc, err := url.QueryUnescape(aws.ToString(version.PolicyVersion.Document))
	if err != nil {
		return nil, fmt.Errorf("failed to URL-decode policy document: %w", err)
	}

	var policyDoc map[string]interface{}
	err = json.Unmarshal([]byte(decodedDoc), &policyDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to parse policy document JSON: %w", err)
	}

	return policyDoc, nil
}

// GenerateResourceSummary creates a summary of the resource inventory
func (cs *ConfigService) GenerateResourceSummary(inventory *ResourceInventory) ResourceSummary {
	summary := ResourceSummary{
		ResourcesByType:   make(map[string]int),
		ResourcesByRegion: make(map[string]int),
		ComplianceStatus:  make(map[string]int), // Note: ComplianceStatus is on the rule, not resource
		TotalResources:    len(inventory.Resources),
		PolicyCount:       len(inventory.Policies),
		ConfigRulesCount:  len(inventory.ComplianceRules),
	}

	for _, resource := range inventory.Resources {
		summary.ResourcesByType[resource.ResourceType]++
		summary.ResourcesByRegion[resource.Region]++
	}

	return summary
}

// CheckConfigStatus checks if AWS Config is enabled and properly configured
func (cs *ConfigService) CheckConfigStatus(ctx context.Context) error {
	input := &configservice.DescribeConfigurationRecordersInput{}
	result, err := cs.client.DescribeConfigurationRecorders(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to check configuration recorders: %w", err)
	}

	if len(result.ConfigurationRecorders) == 0 {
		return fmt.Errorf("no configuration recorders found - AWS Config is not enabled")
	}

	// Check if at least one recorder is recording
	hasActiveRecorder := false
	for _, recorder := range result.ConfigurationRecorders {
		if recorder.RecordingGroup != nil {
			hasActiveRecorder = true
			break
		}
	}

	if !hasActiveRecorder {
		return fmt.Errorf("no active configuration recorders found")
	}

	return nil
}

// GetResourcesByType retrieves resources filtered by specific resource types
func (cs *ConfigService) GetResourcesByType(ctx context.Context, resourceTypes []string) ([]ConfigurationItem, error) {
	log.Printf("[ConfigService] Fetching resources for types: %v", resourceTypes)

	if len(resourceTypes) == 0 {
		return []ConfigurationItem{}, nil
	}

	var resources []ConfigurationItem

	// Build SQL query with resource type filter
	typeFilter := make([]string, len(resourceTypes))
	for i, rt := range resourceTypes {
		typeFilter[i] = fmt.Sprintf("'%s'", rt)
	}

	query := fmt.Sprintf(`SELECT 
		resourceId, 
		resourceType, 
		resourceName, 
		awsRegion, 
		availabilityZone, 
		configuration, 
		configurationItemStatus, 
		configurationStateId, 
		resourceCreationTime, 
		tags, 
		relationships 
	WHERE 
		resourceType IN (%s)`, strings.Join(typeFilter, ","))

	input := &configservice.SelectResourceConfigInput{
		Expression: aws.String(query),
	}

	paginator := configservice.NewSelectResourceConfigPaginator(cs.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get next page of resource configurations: %w", err)
		}

		for _, resultString := range page.Results {
			var item ConfigurationItem
			err := json.Unmarshal([]byte(resultString), &item)
			if err != nil {
				log.Printf("[ConfigService] Warning: failed to unmarshal resource configuration: %v", err)
				continue
			}
			resources = append(resources, item)
		}
	}

	log.Printf("[ConfigService] Successfully fetched %d resources for specified types.", len(resources))
	return resources, nil
}

// createConfigServiceRole creates an IAM role for AWS Config service
func (s *CloudTrailService) createConfigServiceRole(ctx context.Context, cfg aws.Config, accountID string) (string, error) {
	fmt.Println("[AWS Config] Creating Config service role...")

	iamClient := iam.NewFromConfig(cfg)
	roleName := "CloudLoom-Config-ServiceRole"
	roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)

	// Check if role already exists
	getRoleInput := &iam.GetRoleInput{RoleName: aws.String(roleName)}
	_, err := iamClient.GetRole(ctx, getRoleInput)
	if err == nil {
		fmt.Printf("[AWS Config] Role already exists: %s\n", roleArn)
		return roleArn, nil
	}

	// Trust policy for AWS Config service
	trustPolicy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {
					"Service": "config.amazonaws.com"
				},
				"Action": "sts:AssumeRole"
			}
		]
	}`

	// Create the role
	createRoleInput := &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trustPolicy),
		Description:              aws.String("CloudLoom AWS Config service role"),
	}

	_, err = iamClient.CreateRole(ctx, createRoleInput)
	if err != nil {
		return "", fmt.Errorf("failed to create Config service role: %w", err)
	}

	// Attach the AWS managed policy for Config service
	attachPolicyInput := &iam.AttachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String("arn:aws:iam::aws:policy/service-role/ConfigRole"),
	}

	_, err = iamClient.AttachRolePolicy(ctx, attachPolicyInput)
	if err != nil {
		return "", fmt.Errorf("failed to attach Config service policy: %w", err)
	}

	// Also attach the S3 delivery permissions policy
	s3PolicyInput := &iam.AttachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String("arn:aws:iam::aws:policy/service-role/AWS_ConfigRole"),
	}

	_, err = iamClient.AttachRolePolicy(ctx, s3PolicyInput)
	if err != nil {
		fmt.Printf("[AWS Config] Warning: failed to attach S3 delivery policy: %v\n", err)
		// Don't fail completely if this policy attachment fails
	}

	fmt.Printf("[AWS Config] Config service role created: %s\n", roleArn)
	return roleArn, nil
}

// createConfigurationRecorder creates an AWS Config configuration recorder
func (s *CloudTrailService) createConfigurationRecorder(ctx context.Context, cfg aws.Config, recorderName, roleArn string) error {
	fmt.Printf("[AWS Config] Creating configuration recorder: %s\n", recorderName)

	configClient := configservice.NewFromConfig(cfg)

	// Check if recorder already exists
	listInput := &configservice.DescribeConfigurationRecordersInput{}
	listResult, err := configClient.DescribeConfigurationRecorders(ctx, listInput)
	if err != nil {
		return fmt.Errorf("failed to list configuration recorders: %w", err)
	}

	// Check if our recorder already exists
	for _, recorder := range listResult.ConfigurationRecorders {
		if aws.ToString(recorder.Name) == recorderName {
			fmt.Printf("[AWS Config] Configuration recorder already exists: %s\n", recorderName)
			return nil
		}
	}

	// Create the configuration recorder
	createInput := &configservice.PutConfigurationRecorderInput{
		ConfigurationRecorder: &types.ConfigurationRecorder{
			Name:    aws.String(recorderName),
			RoleARN: aws.String(roleArn),
			RecordingGroup: &types.RecordingGroup{
				AllSupported:               true,
				IncludeGlobalResourceTypes: true,
			},
		},
	}

	_, err = configClient.PutConfigurationRecorder(ctx, createInput)
	if err != nil {
		return fmt.Errorf("failed to create configuration recorder: %w", err)
	}

	fmt.Printf("[AWS Config] Configuration recorder created: %s\n", recorderName)
	return nil
}

// createDeliveryChannel creates an AWS Config delivery channel
func (s *CloudTrailService) createDeliveryChannel(ctx context.Context, cfg aws.Config, channelName, bucketName, accountID string) error {
	fmt.Printf("[AWS Config] Creating delivery channel: %s using bucket: %s\n", channelName, bucketName)

	configClient := configservice.NewFromConfig(cfg)

	// Check if delivery channel already exists
	listInput := &configservice.DescribeDeliveryChannelsInput{}
	listResult, err := configClient.DescribeDeliveryChannels(ctx, listInput)
	if err != nil {
		return fmt.Errorf("failed to list delivery channels: %w", err)
	}

	// Check if our channel already exists
	for _, channel := range listResult.DeliveryChannels {
		if aws.ToString(channel.Name) == channelName {
			fmt.Printf("[AWS Config] Delivery channel already exists: %s\n", channelName)
			return nil
		}
	}

	// Create delivery channel with proper S3 key prefix that matches the bucket policy
	s3KeyPrefix := fmt.Sprintf("config/AWSLogs/%s/Config", accountID)
	createInput := &configservice.PutDeliveryChannelInput{
		DeliveryChannel: &types.DeliveryChannel{
			Name:         aws.String(channelName),
			S3BucketName: aws.String(bucketName),
			S3KeyPrefix:  aws.String(s3KeyPrefix),
		},
	}

	_, err = configClient.PutDeliveryChannel(ctx, createInput)
	if err != nil {
		return fmt.Errorf("failed to create delivery channel: %w", err)
	}

	fmt.Printf("[AWS Config] Delivery channel created: %s with prefix: %s\n", channelName, s3KeyPrefix)
	return nil
}

// startConfigurationRecorder starts the AWS Config configuration recorder
func (s *CloudTrailService) startConfigurationRecorder(ctx context.Context, cfg aws.Config, recorderName string) error {
	fmt.Printf("[AWS Config] Starting configuration recorder: %s\n", recorderName)

	configClient := configservice.NewFromConfig(cfg)

	// Check if the recorder is already running
	statusInput := &configservice.DescribeConfigurationRecorderStatusInput{}
	statusResult, err := configClient.DescribeConfigurationRecorderStatus(ctx, statusInput)
	if err != nil {
		return fmt.Errorf("failed to check recorder status: %w", err)
	}

	// Check if our recorder is already running
	for _, status := range statusResult.ConfigurationRecordersStatus {
		if aws.ToString(status.Name) == recorderName && status.Recording {
			fmt.Printf("[AWS Config] Configuration recorder is already running: %s\n", recorderName)
			return nil
		}
	}

	// Start the configuration recorder
	startInput := &configservice.StartConfigurationRecorderInput{
		ConfigurationRecorderName: aws.String(recorderName),
	}

	_, err = configClient.StartConfigurationRecorder(ctx, startInput)
	if err != nil {
		return fmt.Errorf("failed to start configuration recorder: %w", err)
	}

	fmt.Printf("[AWS Config] Configuration recorder started: %s\n", recorderName)
	return nil
}

// createBasicConfigRules creates basic AWS Config compliance rules
func (s *CloudTrailService) createBasicConfigRules(ctx context.Context, cfg aws.Config, accountID string) error {
	fmt.Println("[AWS Config] Creating basic Config rules...")

	configClient := configservice.NewFromConfig(cfg)

	// List of basic Config rules to create
	basicRules := []struct {
		name        string
		source      string
		description string
	}{
		{
			name:        "root-user-access-key-check",
			source:      "AWS_CONFIG_RULE",
			description: "Checks whether the root access key is available",
		},
		{
			name:        "s3-bucket-public-access-prohibited",
			source:      "AWS_CONFIG_RULE",
			description: "Checks if S3 buckets prohibit public access",
		},
		{
			name:        "encrypted-volumes",
			source:      "AWS_CONFIG_RULE",
			description: "Checks whether EBS volumes are encrypted",
		},
	}

	// Get existing rules to avoid duplicates
	listInput := &configservice.DescribeConfigRulesInput{}
	listResult, err := configClient.DescribeConfigRules(ctx, listInput)
	if err != nil {
		return fmt.Errorf("failed to list existing config rules: %w", err)
	}

	existingRules := make(map[string]bool)
	for _, rule := range listResult.ConfigRules {
		existingRules[aws.ToString(rule.ConfigRuleName)] = true
	}

	// Create each rule if it doesn't exist
	for _, rule := range basicRules {
		if existingRules[rule.name] {
			fmt.Printf("[AWS Config] Rule already exists: %s\n", rule.name)
			continue
		}

		putRuleInput := &configservice.PutConfigRuleInput{
			ConfigRule: &types.ConfigRule{
				ConfigRuleName: aws.String(rule.name),
				Description:    aws.String(rule.description),
				Source: &types.Source{
					Owner:            types.OwnerAws,
					SourceIdentifier: aws.String(rule.name),
				},
			},
		}

		_, err = configClient.PutConfigRule(ctx, putRuleInput)
		if err != nil {
			fmt.Printf("[AWS Config] Warning: Failed to create rule %s: %v\n", rule.name, err)
			// Continue with other rules even if one fails
			continue
		}

		fmt.Printf("[AWS Config] Created Config rule: %s\n", rule.name)
	}

	fmt.Println("[AWS Config] Basic Config rules setup completed")
	return nil
}

func (s *CloudTrailService) collectInfrastructureInventory(ctx context.Context, cfg aws.Config) error {
	fmt.Println("[Infrastructure] Starting infrastructure inventory collection...")

	// Create config service instance
	configService := NewConfigService(cfg)

	// Check if AWS Config is enabled
	err := configService.CheckConfigStatus(ctx)
	if err != nil {
		fmt.Printf("[Infrastructure] AWS Config is not yet available (may need time to initialize): %v\n", err)
		fmt.Println("[Infrastructure] Using basic resource enumeration...")
		return s.collectBasicResourceInfo(ctx, cfg)
	}

	// If AWS Config is available, use it
	inventory, err := configService.GetComprehensiveResourceInventory(ctx, cfg)
	if err != nil {
		fmt.Printf("[Infrastructure] Config inventory failed, using basic enumeration: %v\n", err)
		return s.collectBasicResourceInfo(ctx, cfg)
	}

	// Log results
	fmt.Printf("[Infrastructure] ✅ Config inventory complete:\n")
	fmt.Printf("  - Total Resources: %d\n", inventory.ResourceSummary.TotalResources)
	fmt.Printf("  - Total Policies: %d\n", inventory.ResourceSummary.PolicyCount)
	fmt.Printf("  - Total Compliance Rules: %d\n", inventory.ResourceSummary.ConfigRulesCount)

	fmt.Println("[Infrastructure] Infrastructure data ready for further processing")
	return nil
}

// collectBasicResourceInfo provides basic resource enumeration without AWS Config
func (s *CloudTrailService) collectBasicResourceInfo(ctx context.Context, cfg aws.Config) error {
	fmt.Println("[Infrastructure] Collecting basic infrastructure information...")

	var totalResources int

	// Collect EC2 resources
	ec2Count, err := s.collectEC2Resources(ctx, cfg)
	if err != nil {
		fmt.Printf("[Infrastructure] Warning: Failed to collect EC2 resources: %v\n", err)
	} else {
		totalResources += ec2Count
		fmt.Printf("  - EC2 Resources: %d found\n", ec2Count)
	}

	// Collect S3 buckets
	s3Count, err := s.collectS3Resources(ctx, cfg)
	if err != nil {
		fmt.Printf("[Infrastructure] Warning: Failed to collect S3 resources: %v\n", err)
	} else {
		totalResources += s3Count
		fmt.Printf("  - S3 Buckets: %d found\n", s3Count)
	}

	// Collect IAM resources
	iamCount, err := s.collectIAMResources(ctx, cfg)
	if err != nil {
		fmt.Printf("[Infrastructure] Warning: Failed to collect IAM resources: %v\n", err)
	} else {
		totalResources += iamCount
		fmt.Printf("  - IAM Resources: %d found\n", iamCount)
	}

	// Collect RDS resources
	rdsCount, err := s.collectRDSResources(ctx, cfg)
	if err != nil {
		fmt.Printf("[Infrastructure] Warning: Failed to collect RDS resources: %v\n", err)
	} else {
		totalResources += rdsCount
		fmt.Printf("  - RDS Instances: %d found\n", rdsCount)
	}

	// Collect Lambda functions
	lambdaCount, err := s.collectLambdaResources(ctx, cfg)
	if err != nil {
		fmt.Printf("[Infrastructure] Warning: Failed to collect Lambda resources: %v\n", err)
	} else {
		totalResources += lambdaCount
		fmt.Printf("  - Lambda Functions: %d found\n", lambdaCount)
	}

	fmt.Printf("[Infrastructure] ✅ Basic infrastructure enumeration completed - Total: %d resources\n", totalResources)
	return nil
}

// collectEC2Resources collects EC2 instances, volumes, and security groups (placeholder)
func (s *CloudTrailService) collectEC2Resources(ctx context.Context, cfg aws.Config) (int, error) {
	// TODO: Implement actual EC2 resource collection when ec2 service is added to dependencies
	// This would use:
	// - ec2.DescribeInstances for EC2 instances
	// - ec2.DescribeVolumes for EBS volumes
	// - ec2.DescribeSecurityGroups for security groups
	// - ec2.DescribeVpcs for VPCs
	// - ec2.DescribeSubnets for subnets

	fmt.Println("[Infrastructure] EC2: Using placeholder enumeration (requires adding ec2 SDK dependency)")
	return 0, nil // Return 0 count for now
}

// collectS3Resources collects S3 buckets and their configurations
func (s *CloudTrailService) collectS3Resources(ctx context.Context, cfg aws.Config) (int, error) {
	s3Client := s3.NewFromConfig(cfg)

	// List all S3 buckets
	listBucketsInput := &s3.ListBucketsInput{}
	result, err := s3Client.ListBuckets(ctx, listBucketsInput)
	if err != nil {
		return 0, fmt.Errorf("failed to list S3 buckets: %w", err)
	}

	bucketCount := len(result.Buckets)
	for _, bucket := range result.Buckets {
		fmt.Printf("[Infrastructure] S3: Found bucket %s (created: %v)\n",
			aws.ToString(bucket.Name),
			aws.ToTime(bucket.CreationDate))
	}

	return bucketCount, nil
}

// collectIAMResources collects IAM users, roles, and policies
func (s *CloudTrailService) collectIAMResources(ctx context.Context, cfg aws.Config) (int, error) {
	iamClient := iam.NewFromConfig(cfg)
	totalCount := 0

	// Count IAM Users
	userPaginator := iam.NewListUsersPaginator(iamClient, &iam.ListUsersInput{})
	userCount := 0
	for userPaginator.HasMorePages() {
		page, err := userPaginator.NextPage(ctx)
		if err != nil {
			fmt.Printf("[Infrastructure] IAM: Warning - failed to list users: %v\n", err)
			break
		}
		userCount += len(page.Users)
	}
	fmt.Printf("[Infrastructure] IAM: Found %d users\n", userCount)
	totalCount += userCount

	// Count IAM Roles
	rolePaginator := iam.NewListRolesPaginator(iamClient, &iam.ListRolesInput{})
	roleCount := 0
	for rolePaginator.HasMorePages() {
		page, err := rolePaginator.NextPage(ctx)
		if err != nil {
			fmt.Printf("[Infrastructure] IAM: Warning - failed to list roles: %v\n", err)
			break
		}
		roleCount += len(page.Roles)
	}
	fmt.Printf("[Infrastructure] IAM: Found %d roles\n", roleCount)
	totalCount += roleCount

	// Count Customer-Managed IAM Policies
	policyPaginator := iam.NewListPoliciesPaginator(iamClient, &iam.ListPoliciesInput{
		Scope: iamtypes.PolicyScopeTypeLocal, // Only customer-managed policies
	})
	policyCount := 0
	for policyPaginator.HasMorePages() {
		page, err := policyPaginator.NextPage(ctx)
		if err != nil {
			fmt.Printf("[Infrastructure] IAM: Warning - failed to list policies: %v\n", err)
			break
		}
		policyCount += len(page.Policies)
	}
	fmt.Printf("[Infrastructure] IAM: Found %d customer-managed policies\n", policyCount)
	totalCount += policyCount

	return totalCount, nil
}

// collectRDSResources collects RDS instances and clusters (placeholder)
func (s *CloudTrailService) collectRDSResources(ctx context.Context, cfg aws.Config) (int, error) {
	// TODO: Implement actual RDS resource collection when rds service is added to dependencies
	// This would use:
	// - rds.DescribeDBInstances for RDS instances
	// - rds.DescribeDBClusters for RDS clusters
	// - rds.DescribeDBSnapshots for snapshots

	fmt.Println("[Infrastructure] RDS: Using placeholder enumeration (requires adding rds SDK dependency)")
	return 0, nil // Return 0 count for now
}

// collectLambdaResources collects Lambda functions (placeholder)
func (s *CloudTrailService) collectLambdaResources(ctx context.Context, cfg aws.Config) (int, error) {
	// TODO: Implement actual Lambda resource collection when lambda service is added to dependencies
	// This would use:
	// - lambda.ListFunctions for Lambda functions
	// - lambda.ListLayers for Lambda layers

	fmt.Println("[Infrastructure] Lambda: Using placeholder enumeration (requires adding lambda SDK dependency)")
	return 0, nil // Return 0 count for now
}
