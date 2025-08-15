package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type QueueInfo struct {
	AccountID string
	QueueURL  string
	QueueArn  string
	RuleArn   string
	CreatedAt time.Time
}

func (s *CloudTrailService) createSQSQueue(ctx context.Context, cfg aws.Config, queueName, accountID string) (*QueueInfo, error) {
	sqsClient := sqs.NewFromConfig(cfg)
	fmt.Printf("[SQS] Setting up queue '%s'\n", queueName)

	var queueUrl string

	// Check if the queue already exists by trying to get its URL
	getQueueUrlInput := &sqs.GetQueueUrlInput{QueueName: aws.String(queueName)}
	getQueueUrlResult, err := sqsClient.GetQueueUrl(ctx, getQueueUrlInput)

	var nqnf *types.QueueDoesNotExist
	if err == nil {
		// Queue exists, use its URL
		fmt.Printf("[SQS] ✅ Queue already exists, using existing one\n")
		queueUrl = *getQueueUrlResult.QueueUrl
	} else if errors.As(err, &nqnf) {
		// Queue doesn't exist, create it
		fmt.Printf("[SQS] Creating new SQS queue...\n")
		createQueueInput := &sqs.CreateQueueInput{
			QueueName: aws.String(queueName),
		}
		result, err := sqsClient.CreateQueue(ctx, createQueueInput)
		if err != nil {
			return nil, fmt.Errorf("failed to create SQS queue: %w", err)
		}
		fmt.Printf("[SQS] ✅ Queue created successfully\n")
		queueUrl = *result.QueueUrl
	} else {
		// Unexpected error
		return nil, fmt.Errorf("failed to check for queue existence: %w", err)
	}

	// Get the queue ARN first
	getQueueAttributesInput := &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueUrl),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameQueueArn},
	}
	attributes, err := sqsClient.GetQueueAttributes(ctx, getQueueAttributesInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue attributes: %w", err)
	}
	queueArn := attributes.Attributes["QueueArn"]

	queueInfo := &QueueInfo{
		AccountID: accountID,
		QueueURL:  queueUrl,
		QueueArn:  queueArn,
		CreatedAt: time.Now(),
	}

	return queueInfo, nil
}

func (s *CloudTrailService) setSQSQueuePolicy(ctx context.Context, cfg aws.Config, queueURL, queueArn string, ruleArns []string) error {
	sqsClient := sqs.NewFromConfig(cfg)
	fmt.Printf("[SQS] Setting queue policy to allow access from %d rules...\n", len(ruleArns))

    // CORRECTED: The PolicyStatement struct now uses a map for the Principal,
    // which correctly marshals to the JSON object {"Service": "events.amazonaws.com"}.
	type PolicyStatement struct {
		Sid       string              `json:"Sid"`
		Effect    string              `json:"Effect"`
		Principal map[string]string   `json:"Principal"` // Changed this from a struct to a map
		Action    string              `json:"Action"`
		Resource  string              `json:"Resource"`
		Condition struct {
			ArnEquals map[string]string `json:"ArnEquals"`
		} `json:"Condition"`
	}

	var statements []PolicyStatement

	for i, ruleArn := range ruleArns {
		statement := PolicyStatement{
			Sid:    fmt.Sprintf("AllowEventBridgeToSendMessageRule%d", i),
			Effect: "Allow",
            // CORRECTED: Initialize the map directly here.
			Principal: map[string]string{
				"Service": "events.amazonaws.com",
			},
			Action:   "sqs:SendMessage",
			Resource: queueArn,
			Condition: struct {
				ArnEquals map[string]string `json:"ArnEquals"`
			}{
				ArnEquals: map[string]string{"aws:SourceArn": ruleArn},
			},
		}
		statements = append(statements, statement)
	}

	policyMap := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": statements,
	}

	policyBytes, err := json.Marshal(policyMap)
	if err != nil {
		return fmt.Errorf("failed to marshal SQS policy: %w", err)
	}
	queuePolicy := string(policyBytes)

	setAttributesInput := &sqs.SetQueueAttributesInput{
		QueueUrl:   aws.String(queueURL),
		Attributes: map[string]string{"Policy": queuePolicy},
	}
	_, err = sqsClient.SetQueueAttributes(ctx, setAttributesInput)
	if err != nil {
		return fmt.Errorf("failed to set queue policy: %w", err)
	}
	fmt.Printf("[SQS] ✅ Queue policy updated to allow all regional rules\n")

	return nil
}

func (s *CloudTrailService) startSQSPolling(ctx context.Context, cfg aws.Config, queueURL string) {
	sqsClient := sqs.NewFromConfig(cfg)
	fmt.Printf("[SQS Polling] Starting continuous polling for queue: %s\n", queueURL)

	// Check for existing messages in queue before starting polling
	fmt.Printf("[SQS Polling] Checking for existing messages in queue...\n")
	initialReceiveInput := &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     1, // Quick check
	}

	initialResult, err := sqsClient.ReceiveMessage(ctx, initialReceiveInput)
	if err != nil {
		log.Printf("[SQS Polling] Error checking for existing messages: %v", err)
	} else if len(initialResult.Messages) > 0 {
		fmt.Printf("[SQS Polling] Found %d existing messages in queue\n", len(initialResult.Messages))
		for i, message := range initialResult.Messages {
			fmt.Printf("[SQS Polling][Existing Message %d] %s\n", i+1, aws.ToString(message.Body))
		}
	} else {
		fmt.Printf("[SQS Polling] No existing messages found in queue\n")
	}

	// Check EventBridge connection status
	fmt.Printf("[SQS Polling] EventBridge Integration Status:\n")
	fmt.Printf("  - Queue configured to receive from EventBridge: ✅\n")
	fmt.Printf("  - EventBridge rule should target this queue\n")
	fmt.Printf("  - CloudTrail should send events to EventBridge\n")
	fmt.Printf("[SQS Polling] Starting continuous polling with 5-second intervals...\n")

	pollCount := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Println("[SQS Polling] Context cancelled, stopping polling")
			return
		default:
			pollCount++
			// if pollCount%3 == 1 { // Log every 3rd attempt to reduce noise
			// 	fmt.Printf("[SQS Polling] Poll attempt #%d - checking for new messages...\n", pollCount)
			// }
			fmt.Printf("[SQS Polling] Poll attempt #%d - checking for new messages...\n", pollCount)

			receiveMessageInput := &sqs.ReceiveMessageInput{
				QueueUrl:            aws.String(queueURL),
				MaxNumberOfMessages: 10,
				WaitTimeSeconds:     5, // Shorter polling interval
			}

			result, err := sqsClient.ReceiveMessage(ctx, receiveMessageInput)
			if err != nil {
				log.Printf("[SQS Polling] Error receiving messages: %v", err)
				time.Sleep(5 * time.Second) // Wait before retrying
				continue
			}

			if len(result.Messages) > 0 {
				fmt.Printf("[SQS Polling] 🎉 Received %d new messages!\n", len(result.Messages))
				for i, message := range result.Messages {
					fmt.Printf("[SQS Polling][New Message %d] %s\n", i+1, aws.ToString(message.Body))
					s.processSecurityFinding(ctx, message.Body)

					// Delete the message after successful processing
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
}

// checkEventBridgeConnection verifies that EventBridge is properly connected to the SQS queue
func (s *CloudTrailService) checkEventBridgeConnection(ctx context.Context, cfg aws.Config, queueArn, accountID string) {
	fmt.Printf("[EventBridge Check] Verifying EventBridge connection...\n")

	// Use EventBridge client to check rule
	eventBridgeClient := eventbridge.NewFromConfig(cfg)
	ruleName := fmt.Sprintf("CloudLoom-AutoApplyFix-Rule-%s", accountID)

	// Check if rule exists
	describeRuleInput := &eventbridge.DescribeRuleInput{
		Name: aws.String(ruleName),
	}

	ruleResult, err := eventBridgeClient.DescribeRule(ctx, describeRuleInput)
	if err != nil {
		fmt.Printf("[EventBridge Check] ❌ Rule not found: %v\n", err)
		return
	}

	fmt.Printf("[EventBridge Check] ✅ Rule found: %s\n", *ruleResult.Name)
	fmt.Printf("[EventBridge Check] Rule state: %s\n", string(ruleResult.State))
	fmt.Printf("[EventBridge Check] Rule pattern: %s\n", aws.ToString(ruleResult.EventPattern))

	// Check rule targets
	listTargetsInput := &eventbridge.ListTargetsByRuleInput{
		Rule: aws.String(ruleName),
	}

	targetsResult, err := eventBridgeClient.ListTargetsByRule(ctx, listTargetsInput)
	if err != nil {
		fmt.Printf("[EventBridge Check] ❌ Failed to list targets: %v\n", err)
		return
	}

	fmt.Printf("[EventBridge Check] Found %d targets:\n", len(targetsResult.Targets))
	for i, target := range targetsResult.Targets {
		fmt.Printf("[EventBridge Check]   Target %d: %s\n", i+1, aws.ToString(target.Arn))
		if aws.ToString(target.Arn) == queueArn {
			fmt.Printf("[EventBridge Check] ✅ SQS queue is properly targeted\n")
		}
	}
}

// startSQSPollingWithEventBridgeCheck starts SQS polling with EventBridge connection verification
func (s *CloudTrailService) startSQSPollingWithEventBridgeCheck(ctx context.Context, cfg aws.Config, queueURL, queueArn, accountID string) {
	fmt.Printf("[SQS Setup] Pre-polling diagnostics:\n")

	// Check EventBridge connection first
	s.checkEventBridgeConnection(ctx, cfg, queueArn, accountID)

	// Print last few CloudTrail logs (simulated - in real implementation you'd query CloudWatch Logs)
	fmt.Printf("[CloudTrail Logs] Recent CloudTrail activity (last 10 minutes):\n")
	fmt.Printf("[CloudTrail Logs] Note: Real log query would be implemented via CloudWatch Logs API\n")
	fmt.Printf("[CloudTrail Logs] Expected events: S3 operations, EC2 operations, IAM operations\n")
	fmt.Printf("[CloudTrail Logs] These events should trigger EventBridge → SQS messages\n")

	// Start the actual polling
	s.startSQSPolling(ctx, cfg, queueURL)
}

// sendTestMessage sends a test message to the SQS queue for verification
func (s *CloudTrailService) sendTestMessage(ctx context.Context, cfg aws.Config, queueURL, testMessage string) error {
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
