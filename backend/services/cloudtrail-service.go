package services

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type CloudTrailService struct{}

func NewCloudTrailService() *CloudTrailService {
	return &CloudTrailService{}
}

// SetupCloudTrail is the main function to orchestrate the automated setup.
func (s *CloudTrailService) SetupCloudTrail(ctx context.Context) error {
	fmt.Println("=== Starting CloudTrail Setup ===")

	// Get temporary credentials by assuming the customer's role
	fmt.Println("Step 1: Assuming customer role...")
	customerCfg, err := s.assumeRole(ctx)
	if err != nil {
		fmt.Printf("❌ Failed to assume role: %v\n", err)
		return err
	}
	fmt.Println("✅ Successfully assumed customer role")

	// Now, use these temporary credentials to create the necessary resources
	customerRegion := customerCfg.Region // Get the region from the config
	fmt.Printf("Step 2: Using region: %s\n", customerRegion)

	customerAccountID, err := getAccountID(ctx, &customerCfg)
	if err != nil {
		fmt.Printf("❌ Failed to get account ID: %v\n", err)
		return err
	}
	fmt.Printf("✅ Retrieved customer account ID: %s\n", customerAccountID)

	// Generate predictable names for resources (no UUID for reusability)
	// S3 bucket names must be DNS-compliant: lowercase, no underscores, 3-63 characters
	bucketName := fmt.Sprintf("cloudloom-logs-%s", customerAccountID)
	logGroupName := fmt.Sprintf("/aws/cloudtrail/cloudloom-agent-%s", customerAccountID)
	trailName := fmt.Sprintf("CloudLoom-Agent-Trail-%s", customerAccountID)
	queueName := fmt.Sprintf("cloudloom-autoapplyfix-%s", customerAccountID)
	ruleName := fmt.Sprintf("CloudLoom-AutoApplyFix-Rule-%s", customerAccountID)

	fmt.Printf("Step 3: Generated resource names:\n")
	fmt.Printf("  - S3 Bucket: %s\n", bucketName)
	fmt.Printf("  - Log Group: %s\n", logGroupName)
	fmt.Printf("  - Trail: %s\n", trailName)
	fmt.Printf("  - SQS Queue: %s\n", queueName)
	fmt.Printf("  - EventBridge Rule: %s\n", ruleName)

	// Create S3 bucket for CloudTrail logs (reuses existing if found)
	fmt.Println("Step 4: Creating/checking S3 bucket and policy...")
	err = s.createS3BucketAndPolicy(ctx, customerCfg, bucketName, customerAccountID, customerRegion)
	if err != nil {
		fmt.Printf("❌ Failed to create S3 bucket: %v\n", err)
		return fmt.Errorf("failed to create S3 bucket: %w", err)
	}
	fmt.Println("✅ S3 bucket and policy created successfully")

	// Create CloudWatch Logs group and its resource policy
	fmt.Println("Step 5: Creating CloudWatch Log Group...")
	logGroupArn, err := s.createCloudWatchLogGroup(ctx, &customerCfg, logGroupName, customerRegion)
	if err != nil {
		fmt.Printf("❌ Failed to create CloudWatch Log Group: %v\n", err)
		return fmt.Errorf("failed to create CloudWatch Log Group: %w", err)
	}
	fmt.Printf("✅ CloudWatch Log Group created: %s\n", *logGroupArn)

	// Create the IAM role for CloudTrail to write to CloudWatch Logs
	fmt.Println("Step 6: Creating IAM role for CloudTrail...")
	cloudTrailRoleArn, err := s.createCloudTrailIAMRole(ctx, &customerCfg, customerAccountID)
	if err != nil {
		fmt.Printf("❌ Failed to create CloudTrail IAM role: %v\n", err)
		return fmt.Errorf("failed to create CloudTrail IAM role: %w", err)
	}
	fmt.Printf("✅ CloudTrail IAM role created: %s\n", *cloudTrailRoleArn)

	// Create/Update the CloudTrail trail
	fmt.Println("Step 7: Creating/updating CloudTrail trail...")
	err = s.createOrUpdateCloudTrailTrail(ctx, &customerCfg, trailName, bucketName, *logGroupArn, *cloudTrailRoleArn)
	if err != nil {
		fmt.Printf("❌ Failed to create or update CloudTrail: %v\n", err)
		return fmt.Errorf("failed to create or update CloudTrail: %w", err)
	}
	fmt.Println("✅ CloudTrail trail created/updated successfully")

	// Create SQS Queue for Auto Apply Fix (reuses existing if found)
	fmt.Println("Step 8: Creating/checking SQS queue for Auto Apply Fix...")
	queueInfo, err := s.createSQSQueue(ctx, customerCfg, queueName, customerAccountID)
	if err != nil {
		fmt.Printf("❌ Failed to create SQS queue: %v\n", err)
		return fmt.Errorf("failed to create SQS queue: %w", err)
	}
	fmt.Printf("✅ SQS queue ready: %s\n", queueInfo.QueueURL)

	// NEW: Create IAM role for EventBridge to send messages to SQS
	fmt.Println("Step 9: Creating/checking IAM role for EventBridge...")
	eventBridgeRoleArn, err := s.createEventBridgeIAMRole(ctx, &customerCfg, customerAccountID, queueInfo.QueueArn)
	if err != nil {
		return fmt.Errorf("failed to create EventBridge IAM role: %w", err)
	}
	fmt.Printf("✅ EventBridge IAM role created: %s\n", eventBridgeRoleArn)

	regionsToMonitor := []string{"ap-south-1", "us-east-1"} // Add other regions as needed
	fmt.Printf("Step 10: Creating EventBridge rules in regions: %v\n", regionsToMonitor)

	var ruleArns []string
	for _, region := range regionsToMonitor {
		fmt.Printf("--- Processing region: %s ---\n", region)

		// Create a new AWS config targeting the specific region for the API call
		regionalCfg := customerCfg
		regionalCfg.Region = region

		// The rule name can be the same across different regions
		ruleName := fmt.Sprintf("CloudLoom-AutoApplyFix-Rule-%s", customerAccountID)

		// Create the rule, pointing it to the central SQS queue in ap-south-1
		ruleArn, err := s.createEventBridgeRule(ctx, regionalCfg, ruleName, queueInfo.QueueArn, eventBridgeRoleArn)
		if err != nil {
			return fmt.Errorf("❌ failed to create EventBridge rule in region %s: %w", region, err)
		}
		ruleArns = append(ruleArns, ruleArn)
	}
	fmt.Printf("✅ EventBridge rules created successfully.\n")

	// UPDATED: Pass all the collected rule ARNs to the SQS policy function.
	fmt.Println("Step 11: Setting SQS queue policy to allow all rules...")
	err = s.setSQSQueuePolicy(ctx, customerCfg, queueInfo.QueueURL, queueInfo.QueueArn, ruleArns)
	if err != nil {
		return fmt.Errorf("❌ Failed to set SQS queue policy: %w", err)
	}
	fmt.Println("✅ SQS queue policy set successfully")

	// Start SQS polling goroutine with EventBridge connection check
	fmt.Println("Step 12: Starting SQS polling goroutine...")
	go s.startSQSPollingWithEventBridgeCheck(context.Background(), customerCfg, queueInfo.QueueURL, queueInfo.QueueArn, customerAccountID)
	fmt.Println("✅ SQS polling goroutine started")

	fmt.Printf("Step 13: Queue information for reference:\n")
	fmt.Printf("  - Account ID: %s\n", queueInfo.AccountID)
	fmt.Printf("  - Queue URL: %s\n", queueInfo.QueueURL)
	fmt.Printf("  - Queue ARN: %s\n", queueInfo.QueueArn)
	fmt.Printf("  - Rule ARN: %s\n", queueInfo.RuleArn)

	fmt.Println("🎉 CloudTrail and Auto Apply Fix setup completed successfully!")
	return nil
}

// SendTestMessage is an endpoint to test SQS polling functionality
func (s *CloudTrailService) SendTestMessage(ctx context.Context) error {
	fmt.Println("=== Sending Test Message to SQS ===")

	// Get temporary credentials by assuming the customer's role
	fmt.Println("Step 1: Assuming customer role...")
	customerCfg, err := s.assumeRole(ctx)
	if err != nil {
		fmt.Printf("❌ Failed to assume role: %v\n", err)
		return err
	}
	fmt.Println("✅ Successfully assumed customer role")

	customerAccountID, err := getAccountID(ctx, &customerCfg)
	if err != nil {
		fmt.Printf("❌ Failed to get account ID: %v\n", err)
		return err
	}

	queueName := fmt.Sprintf("cloudloom-autoapplyfix-%s", customerAccountID)
	fmt.Printf("Step 2: Using queue name: %s\n", queueName)

	// Get the queue URL
	sqsClient := sqs.NewFromConfig(customerCfg)
	getQueueUrlInput := &sqs.GetQueueUrlInput{QueueName: aws.String(queueName)}
	getQueueUrlResult, err := sqsClient.GetQueueUrl(ctx, getQueueUrlInput)
	if err != nil {
		fmt.Printf("❌ Failed to get queue URL: %v\n", err)
		return err
	}

	queueURL := *getQueueUrlResult.QueueUrl
	fmt.Printf("Step 3: Found queue URL: %s\n", queueURL)

	// Send test message
	testMessage := fmt.Sprintf(`{
        "version": "0",
        "id": "test-event-id",
        "detail-type": "Test Message",
        "source": "cloudloom.test",
        "account": "%s",
        "time": "2024-01-01T12:00:00Z",
        "region": "us-east-1",
        "detail": {
            "eventVersion": "1.05",
            "userIdentity": {
                "type": "Root",
                "principalId": "root",
                "arn": "arn:aws:iam::%s:root",
                "accountId": "%s"
            },
            "eventTime": "2024-01-01T12:00:00Z",
            "eventSource": "test.amazonaws.com",
            "eventName": "TestEvent",
            "sourceIPAddress": "127.0.0.1",
            "userAgent": "CloudLoom-Test"
        }
    }`, customerAccountID, customerAccountID, customerAccountID)

	err = s.sendTestMessage(ctx, customerCfg, queueURL, testMessage)
	if err != nil {
		fmt.Printf("❌ Failed to send test message: %v\n", err)
		return err
	}

	fmt.Println("🎉 Test message sent successfully! Check the polling logs for message reception.")
	return nil
}
