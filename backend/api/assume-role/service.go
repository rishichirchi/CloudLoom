package assumerole

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/google/uuid"
	"github.com/rishichirchi/cloudloom/common"
	awsconfig "github.com/rishichirchi/cloudloom/config"
)

type CloudTrailService struct{}

type QueueInfo struct {
	AccountID string
	QueueURL  string
	QueueArn  string
	RuleArn   string
	CreatedAt time.Time
}

func NewCloudTrailService() *CloudTrailService {
	return &CloudTrailService{}
}

// SetupCloudTrail is the main function to orchestrate the automated setup.
func (s *CloudTrailService) SetupCloudTrail(ctx context.Context) error {
	fmt.Println("=== Starting CloudTrail Setup ===")

	// Get temporary credentials by assuming the customer's role
	fmt.Println("Step 1: Assuming customer role...")
	customerCfg, err := assumeRole(ctx)
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

	// Generate unique names for resources
	// S3 bucket names must be DNS-compliant: lowercase, no underscores, 3-63 characters
	uuidShort := strings.ReplaceAll(uuid.NewString(), "-", "")[:8] // Get first 8 chars of UUID without dashes
	bucketName := fmt.Sprintf("cloudloom-logs-%s-%s", customerAccountID, strings.ToLower(uuidShort))
	logGroupName := fmt.Sprintf("/aws/cloudtrail/cloudloom-agent-%s", customerAccountID)
	trailName := fmt.Sprintf("CloudLoom-Agent-Trail-%s", customerAccountID)
	queueName := fmt.Sprintf("cloudloom-autoapplyfix-%s-%s", customerAccountID, strings.ToLower(uuidShort))
	ruleName := fmt.Sprintf("CloudLoom-AutoApplyFix-Rule-%s", customerAccountID)

	fmt.Printf("Step 3: Generated resource names:\n")
	fmt.Printf("  - S3 Bucket: %s\n", bucketName)
	fmt.Printf("  - Log Group: %s\n", logGroupName)
	fmt.Printf("  - Trail: %s\n", trailName)
	fmt.Printf("  - SQS Queue: %s\n", queueName)
	fmt.Printf("  - EventBridge Rule: %s\n", ruleName)

	// Create S3 bucket for CloudTrail logs
	fmt.Println("Step 4: Creating S3 bucket and policy...")
	err = createS3BucketAndPolicy(ctx, customerCfg, bucketName, customerAccountID, customerRegion)
	if err != nil {
		fmt.Printf("❌ Failed to create S3 bucket: %v\n", err)
		return fmt.Errorf("failed to create S3 bucket: %w", err)
	}
	fmt.Println("✅ S3 bucket and policy created successfully")

	// Create CloudWatch Logs group and its resource policy
	fmt.Println("Step 5: Creating CloudWatch Log Group...")
	logGroupArn, err := createCloudWatchLogGroup(ctx, &customerCfg, logGroupName, customerRegion)
	if err != nil {
		fmt.Printf("❌ Failed to create CloudWatch Log Group: %v\n", err)
		return fmt.Errorf("failed to create CloudWatch Log Group: %w", err)
	}
	fmt.Printf("✅ CloudWatch Log Group created: %s\n", *logGroupArn)

	// Create the IAM role for CloudTrail to write to CloudWatch Logs
	fmt.Println("Step 6: Creating IAM role for CloudTrail...")
	cloudTrailRoleArn, err := createCloudTrailIAMRole(ctx, &customerCfg, customerAccountID)
	if err != nil {
		fmt.Printf("❌ Failed to create CloudTrail IAM role: %v\n", err)
		return fmt.Errorf("failed to create CloudTrail IAM role: %w", err)
	}
	fmt.Printf("✅ CloudTrail IAM role created: %s\n", *cloudTrailRoleArn)

	// Create/Update the CloudTrail trail
	fmt.Println("Step 7: Creating/updating CloudTrail trail...")
	err = createOrUpdateCloudTrailTrail(ctx, &customerCfg, trailName, bucketName, *logGroupArn, *cloudTrailRoleArn)
	if err != nil {
		fmt.Printf("❌ Failed to create or update CloudTrail: %v\n", err)
		return fmt.Errorf("failed to create or update CloudTrail: %w", err)
	}
	fmt.Println("✅ CloudTrail trail created/updated successfully")

	// Create SQS Queue for Auto Apply Fix
	fmt.Println("Step 8: Creating SQS queue for Auto Apply Fix...")
	queueInfo, err := s.createSQSQueue(ctx, customerCfg, queueName, customerAccountID, customerRegion)
	if err != nil {
		fmt.Printf("❌ Failed to create SQS queue: %v\n", err)
		return fmt.Errorf("failed to create SQS queue: %w", err)
	}
	fmt.Printf("✅ SQS queue created: %s\n", queueInfo.QueueURL)

	// Create EventBridge Rule
	fmt.Println("Step 9: Creating EventBridge rule...")
	ruleArn, err := s.createEventBridgeRule(ctx, customerCfg, ruleName, queueInfo.QueueArn, customerAccountID)
	if err != nil {
		fmt.Printf("❌ Failed to create EventBridge rule: %v\n", err)
		return fmt.Errorf("failed to create EventBridge rule: %w", err)
	}
	queueInfo.RuleArn = ruleArn
	fmt.Printf("✅ EventBridge rule created: %s\n", ruleArn)

	// Start SQS polling goroutine
	fmt.Println("Step 10: Starting SQS polling goroutine...")
	go s.startSQSPolling(ctx, customerCfg, queueInfo.QueueURL)
	fmt.Println("✅ SQS polling goroutine started")

	fmt.Printf("Step 11: Queue information for reference:\n")
	fmt.Printf("  - Account ID: %s\n", queueInfo.AccountID)
	fmt.Printf("  - Queue URL: %s\n", queueInfo.QueueURL)
	fmt.Printf("  - Queue ARN: %s\n", queueInfo.QueueArn)
	fmt.Printf("  - Rule ARN: %s\n", queueInfo.RuleArn)

	fmt.Println("🎉 CloudTrail and Auto Apply Fix setup completed successfully!")
	return nil
}

func assumeRole(ctx context.Context) (aws.Config, error) {
	fmt.Println("[AssumeRole] Starting AssumeRole handler")

	stsClient := sts.NewFromConfig(awsconfig.AWSConfig)
	fmt.Println("[AssumeRole] Created STS client")

	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(common.ARNNumber),
		RoleSessionName: aws.String("CloudLoomSession"),
		ExternalId:      aws.String(common.ExternalID),
	}
	fmt.Printf("[AssumeRole] AssumeRoleInput: RoleArn=%s, RoleSessionName=%s, ExternalId=%s\n",
		common.ARNNumber, "CloudLoomSession", common.ExternalID)

	result, err := stsClient.AssumeRole(ctx, assumeRoleInput)
	if err != nil {
		fmt.Printf("[AssumeRole] Failed to assume role: %v\n", err)
		return aws.Config{}, fmt.Errorf("failed to assume role: %w", err)
	}
	fmt.Println("[AssumeRole] Successfully assumed role")

	if result.Credentials == nil {
		fmt.Println("[AssumeRole] Credentials are nil in AssumeRole result")
		return aws.Config{}, fmt.Errorf("assume role succeeded but credentials are nil")
	}

	fmt.Printf("[AssumeRole] Received credentials: AccessKeyId=%s\n", *result.Credentials.AccessKeyId)

	cfg, err := config.LoadDefaultConfig(ctx, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
		*result.Credentials.AccessKeyId,
		*result.Credentials.SecretAccessKey,
		*result.Credentials.SessionToken,
	)), config.WithRegion("ap-south-1"))
	if err != nil {
		fmt.Printf("[AssumeRole] Failed to load AWS config: %v\n", err)
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}
	fmt.Println("[AssumeRole] Successfully loaded AWS config with assumed role credentials")

	return cfg, nil
}

func createS3BucketAndPolicy(ctx context.Context, cfg aws.Config, bucketName, accountID, region string) error {
	fmt.Printf("[S3] Setting up bucket '%s' in region '%s'\n", bucketName, region)

	// Validate bucket name
	if len(bucketName) < 3 || len(bucketName) > 63 {
		return fmt.Errorf("bucket name length must be between 3 and 63 characters, got %d", len(bucketName))
	}

	s3Client := s3.NewFromConfig(cfg)

	// First, check if the bucket already exists
	fmt.Printf("[S3] Checking if bucket already exists...\n")
	_, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})

	bucketExists := (err == nil)
	if bucketExists {
		fmt.Printf("[S3] ✅ Bucket already exists, using existing one\n")
	} else {
		// Create the S3 bucket only if it doesn't exist
		fmt.Printf("[S3] Creating new S3 bucket...\n")

		createBucketInput := &s3.CreateBucketInput{
			Bucket: aws.String(bucketName),
		}

		// Only set CreateBucketConfiguration for regions other than ap-south-1
		if region != "ap-south-1" {
			createBucketInput.CreateBucketConfiguration = &types.CreateBucketConfiguration{
				LocationConstraint: types.BucketLocationConstraint(region),
			}
		}

		_, err := s3Client.CreateBucket(ctx, createBucketInput)
		if err != nil {
			fmt.Printf("[S3] ❌ Failed to create bucket: %v\n", err)
			return err
		}
		fmt.Printf("[S3] ✅ Bucket created successfully\n")
	}

	// Set the bucket policy (this can be updated even if bucket exists)
	fmt.Printf("[S3] Setting bucket policy for CloudTrail access...\n")
	policy := fmt.Sprintf(`{
        "Version": "2012-10-17",
        "Statement": [
            {
                "Sid": "AWSCloudTrailAclCheck20150319",
                "Effect": "Allow",
                "Principal": {"Service": "cloudtrail.amazonaws.com"},
                "Action": "s3:GetBucketAcl",
                "Resource": "arn:aws:s3:::%s"
            },
            {
                "Sid": "AWSCloudTrailWrite20150319",
                "Effect": "Allow",
                "Principal": {"Service": "cloudtrail.amazonaws.com"},
                "Action": "s3:PutObject",
                "Resource": "arn:aws:s3:::%s/AWSLogs/%s/*",
                "Condition": {"StringEquals": {"s3:x-amz-acl": "bucket-owner-full-control"}}
            }
        ]
    }`, bucketName, bucketName, accountID)
	_, err = s3Client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucketName),
		Policy: aws.String(policy),
	})
	if err != nil {
		fmt.Printf("[S3] ❌ Failed to set bucket policy: %v\n", err)
		return err
	}
	fmt.Printf("[S3] ✅ Bucket policy set successfully\n")
	return nil
}

func createCloudWatchLogGroup(ctx context.Context, cfg *aws.Config, logGroupName, region string) (*string, error) {
	fmt.Printf("[CloudWatch] Setting up log group '%s'\n", logGroupName)
	cwlClient := cloudwatchlogs.NewFromConfig(*cfg)

	// First, check if the log group already exists
	fmt.Printf("[CloudWatch] Checking if log group already exists...\n")
	describeResponse, err := cwlClient.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(logGroupName),
	})

	var logGroupExists bool
	if err == nil {
		for _, lg := range describeResponse.LogGroups {
			if lg.LogGroupName != nil && *lg.LogGroupName == logGroupName {
				logGroupExists = true
				fmt.Printf("[CloudWatch] ✅ Log group already exists, using existing one\n")
				break
			}
		}
	}

	// Create the log group only if it doesn't exist
	if !logGroupExists {
		fmt.Printf("[CloudWatch] Creating new log group...\n")
		_, err := cwlClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroupName),
		})
		if err != nil {
			fmt.Printf("[CloudWatch] ❌ Failed to create log group: %v\n", err)
			return nil, err
		}
		fmt.Printf("[CloudWatch] ✅ Log group created successfully\n")
	}

	// Return the ARN of the log group (existing or newly created)
	accountID, err := getAccountID(ctx, cfg)
	if err != nil {
		fmt.Printf("[CloudWatch] ❌ Failed to get account ID for ARN: %v\n", err)
		return nil, err
	}
	logGroupArn := fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s:*", region, accountID, logGroupName)
	fmt.Printf("[CloudWatch] Log group ARN: %s\n", logGroupArn)
	return aws.String(logGroupArn), nil
}

func createCloudTrailIAMRole(ctx context.Context, cfg *aws.Config, accountID string) (*string, error) {
	iamClient := iam.NewFromConfig(*cfg)
	roleName := fmt.Sprintf("CloudLoom-CloudTrail-Role-%s", accountID)
	fmt.Printf("[IAM] Setting up role '%s'\n", roleName)

	// First, check if the role already exists
	fmt.Printf("[IAM] Checking if role already exists...\n")
	getRoleOutput, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})

	var roleArn *string
	if err == nil && getRoleOutput.Role != nil {
		// Role exists, use it
		fmt.Printf("[IAM] ✅ Role already exists, using existing one\n")
		roleArn = getRoleOutput.Role.Arn
		fmt.Printf("[IAM] Existing role ARN: %s\n", *roleArn)
	} else {
		// Role doesn't exist, create it
		fmt.Printf("[IAM] Creating new IAM role...\n")
		assumeRolePolicy := `{
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Principal": {"Service": "cloudtrail.amazonaws.com"},
                "Action": "sts:AssumeRole"
            }
        ]
    }`
		createRoleOutput, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(roleName),
			AssumeRolePolicyDocument: aws.String(assumeRolePolicy),
		})
		if err != nil {
			fmt.Printf("[IAM] ❌ Failed to create role: %v\n", err)
			return nil, err
		}
		fmt.Printf("[IAM] ✅ Role created successfully: %s\n", *createRoleOutput.Role.Arn)
		roleArn = createRoleOutput.Role.Arn
	}

	// Check if the policy is already attached (this can be done regardless of whether role was created or existed)
	policyArn := "arn:aws:iam::aws:policy/CloudWatchLogsFullAccess"
	fmt.Printf("[IAM] Checking if policy is already attached...\n")
	listPoliciesOutput, err := iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(roleName),
	})

	var policyAttached bool
	if err == nil {
		for _, policy := range listPoliciesOutput.AttachedPolicies {
			if policy.PolicyArn != nil && *policy.PolicyArn == policyArn {
				policyAttached = true
				fmt.Printf("[IAM] ✅ Policy already attached\n")
				break
			}
		}
	}

	// Attach the policy only if it's not already attached
	if !policyAttached {
		fmt.Printf("[IAM] Attaching policy '%s' to role...\n", policyArn)
		_, err = iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(policyArn),
		})
		if err != nil {
			fmt.Printf("[IAM] ❌ Failed to attach policy: %v\n", err)
			return nil, err
		}
		fmt.Printf("[IAM] ✅ Policy attached successfully\n")

		// Give some time for the role to become available (propagation delay) only for new attachments
		fmt.Printf("[IAM] Waiting 10 seconds for role propagation...\n")
		time.Sleep(10 * time.Second)
		fmt.Printf("[IAM] ✅ Role propagation complete\n")
	}

	return roleArn, nil
}

func createOrUpdateCloudTrailTrail(ctx context.Context, cfg *aws.Config, trailName, bucketName, logGroupArn, cloudTrailRoleArn string) error {
	cloudTrailClient := cloudtrail.NewFromConfig(*cfg)
	fmt.Printf("[CloudTrail] Setting up trail '%s'\n", trailName)

	// First, check if the trail already exists
	fmt.Printf("[CloudTrail] Checking if trail already exists...\n")
	describeOutput, err := cloudTrailClient.DescribeTrails(ctx, &cloudtrail.DescribeTrailsInput{
		TrailNameList: []string{trailName},
	})

	if err == nil && len(describeOutput.TrailList) > 0 {
		// Trail exists, so update it
		fmt.Printf("[CloudTrail] Trail exists, updating...\n")
		_, err = cloudTrailClient.UpdateTrail(ctx, &cloudtrail.UpdateTrailInput{
			Name:                       aws.String(trailName),
			S3BucketName:               aws.String(bucketName),
			CloudWatchLogsLogGroupArn:  aws.String(logGroupArn),
			CloudWatchLogsRoleArn:      aws.String(cloudTrailRoleArn),
			IsMultiRegionTrail:         aws.Bool(true),
			IncludeGlobalServiceEvents: aws.Bool(true),
		})
		if err != nil {
			fmt.Printf("[CloudTrail] ❌ Failed to update trail: %v\n", err)
			return err
		}
		fmt.Printf("[CloudTrail] ✅ Trail updated successfully\n")
	} else {
		// Trail does not exist, so create it
		fmt.Printf("[CloudTrail] Trail does not exist, creating new trail...\n")
		_, err = cloudTrailClient.CreateTrail(ctx, &cloudtrail.CreateTrailInput{
			Name:                       aws.String(trailName),
			S3BucketName:               aws.String(bucketName),
			CloudWatchLogsLogGroupArn:  aws.String(logGroupArn),
			CloudWatchLogsRoleArn:      aws.String(cloudTrailRoleArn),
			IsMultiRegionTrail:         aws.Bool(true),
			IncludeGlobalServiceEvents: aws.Bool(true),
		})
		if err != nil {
			fmt.Printf("[CloudTrail] ❌ Failed to create trail: %v\n", err)
			return err
		}
		fmt.Printf("[CloudTrail] ✅ Trail created successfully\n")
	}

	fmt.Printf("[CloudTrail] Trail configuration:\n")
	fmt.Printf("  - S3 Bucket: %s\n", bucketName)
	fmt.Printf("  - Log Group ARN: %s\n", logGroupArn)
	fmt.Printf("  - Role ARN: %s\n", cloudTrailRoleArn)
	fmt.Printf("  - Multi-Region: true\n")
	fmt.Printf("  - Global Service Events: true\n")

	return err
}

// Helper to get account ID from config using STS
func getAccountID(ctx context.Context, cfg *aws.Config) (string, error) {
	fmt.Printf("[STS] Getting account ID...\n")
	stsClient := sts.NewFromConfig(*cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		fmt.Printf("[STS] ❌ Failed to get caller identity: %v\n", err)
		return "", fmt.Errorf("failed to get caller identity: %w", err)
	}
	fmt.Printf("[STS] ✅ Account ID retrieved: %s\n", *identity.Account)
	fmt.Printf("[STS] Caller ARN: %s\n", *identity.Arn)
	return *identity.Account, nil
}

func (s *CloudTrailService) createSQSQueue(ctx context.Context, cfg aws.Config, queueName, accountID, region string) (*QueueInfo, error) {
	sqsClient := sqs.NewFromConfig(cfg)

	// Create the queue
	createQueueInput := &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
		Attributes: map[string]string{
			"VisibilityTimeoutSeconds":      "300",
			"MessageRetentionPeriod":        "1209600", // 14 days
			"ReceiveMessageWaitTimeSeconds": "20",      // Enable long polling
		},
	}

	result, err := sqsClient.CreateQueue(ctx, createQueueInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQS queue: %w", err)
	}

	// Get queue attributes to retrieve ARN
	getQueueAttributesInput := &sqs.GetQueueAttributesInput{
		QueueUrl:       result.QueueUrl,
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	}

	attributes, err := sqsClient.GetQueueAttributes(ctx, getQueueAttributesInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue attributes: %w", err)
	}

	queueArn := attributes.Attributes["QueueArn"]

	queueInfo := &QueueInfo{
		AccountID: accountID,
		QueueURL:  *result.QueueUrl,
		QueueArn:  queueArn,
		CreatedAt: time.Now(),
	}

	return queueInfo, nil
}

func (s *CloudTrailService) createEventBridgeRule(ctx context.Context, cfg aws.Config, ruleName, queueArn, accountID string) (string, error) {
	eventBridgeClient := eventbridge.NewFromConfig(cfg)

	// Create the rule
	createRuleInput := &eventbridge.PutRuleInput{
		Name:        aws.String(ruleName),
		Description: aws.String("CloudLoom Auto Apply Fix rule for security findings"),
		EventPattern: aws.String(`{
			"source": ["aws.securityhub", "aws.guardduty", "aws.inspector2"],
			"detail-type": ["Security Hub Findings - Imported", "GuardDuty Finding", "Inspector2 Finding"]
		}`),
		State: ebtypes.RuleStateEnabled,
	}

	ruleResult, err := eventBridgeClient.PutRule(ctx, createRuleInput)
	if err != nil {
		return "", fmt.Errorf("failed to create EventBridge rule: %w", err)
	}

	// Add the SQS queue as a target
	putTargetsInput := &eventbridge.PutTargetsInput{
		Rule: aws.String(ruleName),
		Targets: []ebtypes.Target{
			{
				Id:  aws.String("1"),
				Arn: aws.String(queueArn),
				SqsParameters: &ebtypes.SqsParameters{
					MessageGroupId: aws.String("cloudloom-autoapplyfix"),
				},
			},
		},
	}

	_, err = eventBridgeClient.PutTargets(ctx, putTargetsInput)
	if err != nil {
		return "", fmt.Errorf("failed to add targets to EventBridge rule: %w", err)
	}

	return *ruleResult.RuleArn, nil
}

func (s *CloudTrailService) startSQSPolling(ctx context.Context, cfg aws.Config, queueURL string) {
	sqsClient := sqs.NewFromConfig(cfg)
	fmt.Printf("[SQS Polling] Starting continuous polling for queue: %s\n", queueURL)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("[SQS Polling] Context cancelled, stopping polling")
			return
		default:
			// Poll for messages
			receiveMessageInput := &sqs.ReceiveMessageInput{
				QueueUrl:            aws.String(queueURL),
				MaxNumberOfMessages: 10,
				WaitTimeSeconds:     20, // Long polling
			}

			result, err := sqsClient.ReceiveMessage(ctx, receiveMessageInput)
			if err != nil {
				log.Printf("[SQS Polling] Error receiving messages: %v", err)
				time.Sleep(30 * time.Second) // Wait before retrying
				continue
			}

			if len(result.Messages) > 0 {
				fmt.Printf("[SQS Polling] Received %d messages\n", len(result.Messages))

				for _, message := range result.Messages {
					// Process the message
					s.processSecurityFinding(ctx, message.Body)

					// Delete the message after processing
					deleteMessageInput := &sqs.DeleteMessageInput{
						QueueUrl:      aws.String(queueURL),
						ReceiptHandle: message.ReceiptHandle,
					}

					_, err := sqsClient.DeleteMessage(ctx, deleteMessageInput)
					if err != nil {
						log.Printf("[SQS Polling] Error deleting message: %v", err)
					}
				}
			}
		}
	}
}

func (s *CloudTrailService) processSecurityFinding(ctx context.Context, messageBody *string) {
	if messageBody == nil {
		return
	}

	fmt.Printf("[Security Finding] Processing security finding: %s\n", *messageBody)
	// TODO: Implement security finding processing logic
	// This would include:
	// 1. Parse the finding
	// 2. Determine the appropriate fix
	// 3. Apply the fix automatically
	// 4. Log the action taken
}

// SendTestMessage is an endpoint to test SQS polling functionality
func (s *CloudTrailService) SendTestMessage(ctx context.Context) error {
	fmt.Println("=== Sending Test Message to SQS ===")

	// Get temporary credentials by assuming the customer's role
	fmt.Println("Step 1: Assuming customer role...")
	customerCfg, err := assumeRole(ctx)
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

	err = sendTestMessage(ctx, customerCfg, queueURL, testMessage)
	if err != nil {
		fmt.Printf("❌ Failed to send test message: %v\n", err)
		return err
	}

	fmt.Println("🎉 Test message sent successfully! Check the polling logs for message reception.")
	return nil
}

// sendTestMessage sends a test message to the SQS queue for verification
func sendTestMessage(ctx context.Context, cfg aws.Config, queueURL, testMessage string) error {
	sqsClient := sqs.NewFromConfig(cfg)
	fmt.Printf("[SQS Test] Sending test message to queue...\n")

	sendMessageInput := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(testMessage),
	}

	result, err := sqsClient.SendMessage(ctx, sendMessageInput)
	if err != nil {
		return fmt.Errorf("failed to send test message: %w", err)
	}

	fmt.Printf("[SQS Test] ✅ Test message sent successfully. Message ID: %s\n", *result.MessageId)
	return nil
}
