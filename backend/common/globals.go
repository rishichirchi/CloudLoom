package common

// AWS Role Configuration
var ARNNumber = "arn:aws:iam::980921722037:role/CloudLoomAutoApplyFixRole"
var ExternalID = "cloudloom-7132a5d5-7ce1-4c8e-aad2-af58105606e6"
var GithubRepoLink *string

// AWS Temporary Credentials (populated after assuming role)
var (
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	AWSSessionToken    string
	AWSRegion          string
	IsCredentialsSet   bool
)

// SetAWSCredentials sets the global AWS credentials after role assumption
func SetAWSCredentials(accessKey, secretKey, sessionToken, region string) {
	AWSAccessKeyID = accessKey
	AWSSecretAccessKey = secretKey
	AWSSessionToken = sessionToken
	AWSRegion = region
	IsCredentialsSet = true
}

// ClearAWSCredentials clears the global AWS credentials
func ClearAWSCredentials() {
	AWSAccessKeyID = ""
	AWSSecretAccessKey = ""
	AWSSessionToken = ""
	AWSRegion = ""
	IsCredentialsSet = false
}

// HasValidCredentials checks if AWS credentials are set and valid
func HasValidCredentials() bool {
	return IsCredentialsSet &&
		AWSAccessKeyID != "" &&
		AWSSecretAccessKey != "" &&
		AWSRegion != ""
}
