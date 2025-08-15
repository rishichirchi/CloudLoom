package services

import (
    "context"
    "fmt"
    "errors"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
    cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
    "github.com/aws/aws-sdk-go-v2/service/sts"
)

// createCloudWatchLogGroup creates or checks for an existing log group and sets its policy.
func (s *CloudTrailService) createCloudWatchLogGroup(ctx context.Context, cfg *aws.Config, logGroupName, region string) (*string, error) {
    fmt.Printf("[CloudWatch] Setting up log group '%s'\n", logGroupName)
    cwlClient := cloudwatchlogs.NewFromConfig(*cfg)

    var logGroupArn string

    // Check if the log group already exists.
    // DescribeLogGroups with a prefix returns an empty list if not found, not an error.
    describeOutput, err := cwlClient.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
        LogGroupNamePrefix: aws.String(logGroupName),
    })
    if err != nil {
        return nil, fmt.Errorf("failed to describe log groups: %w", err)
    }

    found := false
    for _, lg := range describeOutput.LogGroups {
        if lg.LogGroupName != nil && *lg.LogGroupName == logGroupName {
            fmt.Printf("[CloudWatch] ✅ Log group '%s' already exists.\n", logGroupName)
            logGroupArn = *lg.Arn
            found = true
            break
        }
    }

    // If the log group was not found, create it.
    if !found {
        fmt.Printf("[CloudWatch] Log group not found. Creating new log group '%s'...\n", logGroupName)
        _, err := cwlClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
            LogGroupName: aws.String(logGroupName),
        })
        if err != nil {
            return nil, fmt.Errorf("failed to create log group: %w", err)
        }
        fmt.Printf("[CloudWatch] ✅ Log group created successfully.\n")
    }

    // Get Account ID for constructing ARNs
    accountID, err := getAccountID(ctx, cfg)
    if err != nil {
        return nil, fmt.Errorf("failed to get account ID: %w", err)
    }

    // If we just created the group, we need to construct its ARN.
    // The actual resource ARN does NOT have a wildcard at the end.
    if logGroupArn == "" {
        logGroupArn = fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s", region, accountID, logGroupName)
    }
    
    fmt.Printf("[CloudWatch] Log group resource ARN: %s\n", logGroupArn)

    // Set the resource policy to allow CloudTrail to write to the log group.
    // The ARN used inside the policy document *does* need the "/*" wildcard.
    policyResourceArn := logGroupArn + ":*"
    err = s.setCloudWatchLogGroupPolicy(ctx, cfg, policyResourceArn, accountID)
    if err != nil {
        return nil, fmt.Errorf("failed to set log group policy: %w", err)
    }
    fmt.Printf("[CloudWatch] ✅ Log group policy set successfully.\n")

    // Return the CORRECT log group ARN (without the wildcard).
    return aws.String(logGroupArn), nil
}

// setCloudWatchLogGroupPolicy sets the policy on the log group to allow CloudTrail access.
// This function is slightly modified to not require redundant parameters.
func (s *CloudTrailService) setCloudWatchLogGroupPolicy(ctx context.Context, cfg *aws.Config, policyResourceArn, accountID string) error {
    cwlClient := cloudwatchlogs.NewFromConfig(*cfg)

    policyName := "CloudLoom-CloudTrail-Access-Policy"
    policyDocument := fmt.Sprintf(`{
        "Version": "2012-10-17",
        "Statement": [
            {
                "Sid": "AWSCloudTrailWrite20150319",
                "Effect": "Allow",
                "Principal": {"Service": "cloudtrail.amazonaws.com"},
                "Action": "logs:PutLogEvents",
                "Resource": "%s",
                "Condition": {
                    "StringEquals": {
                        "aws:SourceArn": "arn:aws:cloudtrail:%s:%s:trail/*"
                    }
                }
            }
        ]
    }`, policyResourceArn, cfg.Region, accountID)

    // Note: PutResourcePolicy can sometimes return an error if you try to apply the same policy again.
    // In a real-world scenario, you might want to call DescribeResourcePolicies first.
    // For this use case, overwriting is generally fine.
    _, err := cwlClient.PutResourcePolicy(ctx, &cloudwatchlogs.PutResourcePolicyInput{
        PolicyName:     aws.String(policyName),
        PolicyDocument: aws.String(policyDocument),
    })

    var alreadyExistsEx *cwltypes.InvalidParameterException
    if err != nil && errors.As(err, &alreadyExistsEx) && *alreadyExistsEx.Message == "Policy with the same name already exists." {
        fmt.Printf("[CloudWatch] ℹ️ Resource policy '%s' already exists. No update needed.\n", policyName)
        return nil // Not an error in this context
    }

    return err
}

// getAccountID helper remains the same.
func getAccountID(ctx context.Context, cfg *aws.Config) (string, error) {
    fmt.Printf("[STS] Getting account ID...\n")
    stsClient := sts.NewFromConfig(*cfg)
    identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
    if err != nil {
        fmt.Printf("[STS] ❌ Failed to get caller identity: %v\n", err)
        return "", fmt.Errorf("failed to get caller identity: %w", err)
    }
    fmt.Printf("[STS] ✅ Account ID retrieved: %s\n", *identity.Account)
    return *identity.Account, nil
}