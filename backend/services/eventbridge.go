// eventbridge.go
package services

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/eventbridge"
    ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
    "github.com/aws/aws-sdk-go-v2/service/iam"
)

func (s *CloudTrailService) createEventBridgeRule(ctx context.Context, cfg aws.Config, ruleName, queueArn, eventBridgeRoleArn string) (string, error) {
    eventBridgeClient := eventbridge.NewFromConfig(cfg)
    fmt.Printf("[EventBridge] Setting up rule '%s'\n", ruleName)

    // FIXED: A more robust and simpler event pattern.
    // This captures all API calls from key services without needing a long, static list of event names.
    // This is much more likely to catch the events you care about.
    eventPattern := `{
        "source": ["aws.s3", "aws.ec2", "aws.iam", "aws.rds", "aws.cloudformation"],
        "detail-type": ["AWS API Call via CloudTrail"]
    }`

    putRuleInput := &eventbridge.PutRuleInput{
        Name:         aws.String(ruleName),
        Description:  aws.String("CloudLoom Auto Apply Fix rule for AWS API events"),
        EventPattern: aws.String(eventPattern),
        State:        ebtypes.RuleStateEnabled,
    }

    ruleResult, err := eventBridgeClient.PutRule(ctx, putRuleInput)
    if err != nil {
        return "", fmt.Errorf("failed to create or update EventBridge rule: %w", err)
    }
    fmt.Printf("[EventBridge] ✅ Rule created/updated successfully: %s\n", *ruleResult.RuleArn)

    // Add SQS queue as the target
    fmt.Printf("[EventBridge] Adding/updating SQS target...\n")
    putTargetsInput := &eventbridge.PutTargetsInput{
        Rule: aws.String(ruleName),
        Targets: []ebtypes.Target{
            {
                Id:      aws.String("CloudLoom-SQS-Target"), // A more descriptive ID
                Arn:     aws.String(queueArn),
                RoleArn: aws.String(eventBridgeRoleArn),
            },
        },
    }

    _, err = eventBridgeClient.PutTargets(ctx, putTargetsInput)
    if err != nil {
        return "", fmt.Errorf("failed to add targets to EventBridge rule: %w", err)
    }
    fmt.Printf("[EventBridge] ✅ Target added/updated successfully\n")

    return *ruleResult.RuleArn, nil
}

func (s *CloudTrailService) createEventBridgeIAMRole(ctx context.Context, cfg *aws.Config, accountID string, queueArn string) (string, error) {
    iamClient := iam.NewFromConfig(*cfg)
    roleName := fmt.Sprintf("CloudLoom-Events-Role-%s", accountID)
    policyName := fmt.Sprintf("CloudLoom-EventBridge-SQSPolicy-%s", accountID)

    // Check if role exists
    getRoleOutput, err := iamClient.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(roleName)})
    if err == nil && getRoleOutput.Role != nil {
        log.Printf("[IAM] ✅ EventBridge Role '%s' already exists.", roleName)
        // Ensure policy is up-to-date even if role exists
    } else {
        log.Printf("[IAM] Creating new IAM role '%s' for EventBridge", roleName)
        assumeRolePolicy := `{
            "Version": "2012-10-17",
            "Statement": [{"Effect": "Allow", "Principal": {"Service": "events.amazonaws.com"}, "Action": "sts:AssumeRole"}]
        }`
        _, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
            RoleName:                 aws.String(roleName),
            AssumeRolePolicyDocument: aws.String(assumeRolePolicy),
        })
        if err != nil {
            return "", fmt.Errorf("failed to create EventBridge IAM role: %w", err)
        }
    }
    
    // FIXED: Use a specific policy that ONLY allows sending to the created SQS queue.
    policyDocument := fmt.Sprintf(`{
        "Version": "2012-10-17",
        "Statement": [{
            "Effect": "Allow",
            "Action": "sqs:SendMessage",
            "Resource": "%s"
        }]
    }`, queueArn)
    
    _, err = iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
        RoleName:       aws.String(roleName),
        PolicyName:     aws.String(policyName),
        PolicyDocument: aws.String(policyDocument),
    })
    if err != nil {
        return "", fmt.Errorf("failed to attach SQS SendMessage policy to EventBridge role: %w", err)
    }
    
    // Give some time for role to propagate
    time.Sleep(10 * time.Second)

    // Return the constructed role ARN
    roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)
    return roleArn, nil
}