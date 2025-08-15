package services

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func (s *CloudTrailService) createS3BucketAndPolicy(ctx context.Context, cfg aws.Config, bucketName, accountID, region string) error {
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
			CreateBucketConfiguration: &types.CreateBucketConfiguration{
				LocationConstraint: types.BucketLocationConstraint("ap-south-1"),
			},
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
