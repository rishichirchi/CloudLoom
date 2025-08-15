package services

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/rishichirchi/cloudloom/common"
	awsconfig "github.com/rishichirchi/cloudloom/config"
)

func (s *CloudTrailService) assumeRole(ctx context.Context) (aws.Config, error) {
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
