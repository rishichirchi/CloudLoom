package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

func (s *CloudTrailService) createCloudTrailIAMRole(ctx context.Context, cfg *aws.Config, accountID string) (*string, error) {
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

func (s *CloudTrailService) createOrUpdateCloudTrailTrail(ctx context.Context, cfg *aws.Config, trailName, bucketName, logGroupArn, cloudTrailRoleArn string) error {
	cloudTrailClient := cloudtrail.NewFromConfig(*cfg)
	fmt.Printf("[CloudTrail] Setting up trail '%s'\n", trailName)

	// First, check if the trail already exists
	fmt.Printf("[CloudTrail] Checking if trail already exists...\n")
	describeOutput, err := cloudTrailClient.DescribeTrails(ctx, &cloudtrail.DescribeTrailsInput{
		TrailNameList: []string{trailName},
	})

	var trailExists bool
	if err == nil && len(describeOutput.TrailList) > 0 {
		trailExists = true
		fmt.Printf("[CloudTrail] Trail found via DescribeTrails\n")
	}

	if trailExists {
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
		// Trail does not exist according to DescribeTrails, attempt to create it
		fmt.Printf("[CloudTrail] Trail not found via DescribeTrails, attempting to create...\n")
		_, err = cloudTrailClient.CreateTrail(ctx, &cloudtrail.CreateTrailInput{
			Name:                       aws.String(trailName),
			S3BucketName:               aws.String(bucketName),
			CloudWatchLogsLogGroupArn:  aws.String(logGroupArn),
			CloudWatchLogsRoleArn:      aws.String(cloudTrailRoleArn),
			IsMultiRegionTrail:         aws.Bool(true),
			IncludeGlobalServiceEvents: aws.Bool(true),
		})
		if err != nil {
			// Check if the error is because the trail already exists
			if strings.Contains(err.Error(), "TrailAlreadyExistsException") {
				fmt.Printf("[CloudTrail] Trail already exists (caught exception), attempting to update instead...\n")
				// Try to update the existing trail
				_, updateErr := cloudTrailClient.UpdateTrail(ctx, &cloudtrail.UpdateTrailInput{
					Name:                       aws.String(trailName),
					S3BucketName:               aws.String(bucketName),
					CloudWatchLogsLogGroupArn:  aws.String(logGroupArn),
					CloudWatchLogsRoleArn:      aws.String(cloudTrailRoleArn),
					IsMultiRegionTrail:         aws.Bool(true),
					IncludeGlobalServiceEvents: aws.Bool(true),
				})
				if updateErr != nil {
					fmt.Printf("[CloudTrail] ❌ Failed to update existing trail: %v\n", updateErr)
					return updateErr
				}
				fmt.Printf("[CloudTrail] ✅ Existing trail updated successfully\n")
			} else {
				fmt.Printf("[CloudTrail] ❌ Failed to create trail: %v\n", err)
				return err
			}
		} else {
			fmt.Printf("[CloudTrail] ✅ Trail created successfully\n")
		}
	}

	fmt.Printf("[CloudTrail] Trail configuration:\n")
	fmt.Printf("  - S3 Bucket: %s\n", bucketName)
	fmt.Printf("  - Log Group ARN: %s\n", logGroupArn)
	fmt.Printf("  - Role ARN: %s\n", cloudTrailRoleArn)
	fmt.Printf("  - Multi-Region: true\n")
	fmt.Printf("  - Global Service Events: true\n")

	// IMPORTANT: Start logging for the trail
	fmt.Printf("[CloudTrail] Starting logging for trail...\n")
	_, err = cloudTrailClient.StartLogging(ctx, &cloudtrail.StartLoggingInput{
		Name: aws.String(trailName),
	})
	if err != nil {
		fmt.Printf("[CloudTrail] ❌ Failed to start logging: %v\n", err)
		return err
	}
	fmt.Printf("[CloudTrail] ✅ Trail logging started successfully\n")

	return nil
}
