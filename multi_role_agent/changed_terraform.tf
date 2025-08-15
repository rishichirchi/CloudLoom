# Configure the AWS Provider
provider "aws" {
  region = "us-east-1" # Example region
}

# --- Networking Infrastructure ---

# 1. Create a Virtual Private Cloud (VPC)
resource "aws_vpc" "app_vpc" {
  cidr_block = "10.10.0.0/16"
  tags = {
    Name = "app-network-vpc"
  }
}

# 2. Create a Subnet within the VPC
resource "aws_subnet" "app_subnet_public" {
  vpc_id     = aws_vpc.app_vpc.id # Depends on VPC
  cidr_block = "10.10.1.0/24"
  availability_zone = "us-east-1a"
  map_public_ip_on_launch = true # Make it a public subnet for example instance
  tags = {
    Name = "app-public-subnet"
  }
}

# 3. Create an Internet Gateway and attach it to the VPC for public access
resource "aws_internet_gateway" "app_igw" {
  vpc_id = aws_vpc.app_vpc.id # Depends on VPC
  tags = {
    Name = "app-igw"
  }
}

# 4. Create a Route Table for the public subnet
resource "aws_route_table" "app_public_rt" {
  vpc_id = aws_vpc.app_vpc.id # Depends on VPC
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.app_igw.id # Depends on Internet Gateway
  }
  tags = {
    Name = "app-public-rt"
  }
}

# 5. Associate the public subnet with the public route table
resource "aws_route_table_association" "app_public_rta" {
  subnet_id      = aws_subnet.app_subnet_public.id      # Depends on Subnet
  route_table_id = aws_route_table.app_public_rt.id # Depends on Route Table
}

# 6. Create a Security Group for the EC2 instance
resource "aws_security_group" "app_instance_sg" {
  name        = "app-instance-sg"
  description = "Allow web access and SSH"
  vpc_id      = aws_vpc.app_vpc.id # Depends on VPC

  ingress {
    description = "Allow HTTP"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["192.168.1.1/32"] # Restrict to a specific IP - REPLACE WITH YOUR IP
  }
  ingress {
    description = "Allow SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["192.168.1.1/32"] # Restrict to a specific IP - REPLACE WITH YOUR IP
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = {
    Name = "app-instance-sg"
  }
}

# --- S3 Service ---

# 7. Create an S3 Bucket
# The EC2 instance will need permission to access this bucket.
resource "aws_s3_bucket" "app_data_bucket" {
  bucket = "my-unique-app-data-bucket-example-12345" # S3 bucket names must be globally unique
  acl    = "private" # Keep it private by default

  logging {
    target_bucket = aws_s3_bucket.app_logs_bucket.id
    target_prefix = "log/"
  }

  versioning {
    enabled = true
  }

  tags = {
    Name = "App Data Bucket"
  }
}

resource "aws_s3_bucket" "app_logs_bucket" {
  bucket = "my-unique-app-logs-bucket"
  acl    = "private"

  tags = {
    Name = "App Logs Bucket"
  }
}

# --- IAM for EC2 to access S3 ---

# 8. Define an IAM Policy that grants S3 permissions
# This policy allows read/write access to objects in the specific S3 bucket.
resource "aws_iam_policy" "s3_read_write_policy" {
  name        = "S3ReadWritePolicyForAppInstance"
  description = "Allows EC2 instance to read and write objects in the app data bucket"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:ListBucket" # Permission to list objects in the bucket
        ]
        # This is the key part linking the permission to the bucket
        Resource = [
          aws_s3_bucket.app_data_bucket.arn, # Permission on the bucket itself (for ListBucket)
          "${aws_s3_bucket.app_data_bucket.arn}/*" # Permission on objects within the bucket (for Get/Put/DeleteObject)
        ]
      },
    ]
  })
}

# 9. Define an IAM Role that EC2 instances can assume
# This trust policy allows the EC2 service to assume this role.
resource "aws_iam_role" "app_instance_role" {
  name = "AppInstanceRole"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      },
    ]
  })
}

# 10. Attach the S3 policy to the EC2 Role
# Dependency: The policy attachment depends on the policy and the role existing.
resource "aws_iam_role_policy_attachment" "app_role_policy_attach" {
  role       = aws_iam_role.app_instance_role.name           # Depends on the Role
  policy_arn = aws_iam_policy.s3_read_write_policy.arn # Depends on the Policy
}

# 11. Create an IAM Instance Profile
# EC2 instances assume roles via an Instance Profile.
# Dependency: The instance profile depends on the role.
resource "aws_iam_instance_profile" "app_instance_profile" {
  name = "AppInstanceProfile"
  role = aws_iam_role.app_instance_role.name # Depends on the Role
}


# --- Compute Service ---

# 12. Create the EC2 Instance
# This instance references the subnet, security group, and the IAM Instance Profile.
# Dependencies: The instance depends on the subnet, security group, and instance profile.
resource "aws_instance" "app_server" {
  ami                    = "ami-053b0d53c279c366f" # Example AMI ID (Amazon Linux 2 in us-east-1) - replace with a valid one
  instance_type          = "t2.micro"
  subnet_id              = aws_subnet.app_subnet_public.id # Depends on Subnet
  vpc_security_group_ids = [aws_security_group.app_instance_sg.id] # Depends on Security Group
  iam_instance_profile   = aws_iam_instance_profile.app_instance_profile.name # Depends on Instance Profile

  tags = {
    Name = "app-web-server"
  }
}

# --- Outputs ---

# 13. Output the Public IP of the instance
output "app_server_public_ip" {
  description = "Public IP address of the app web server"
  value       = aws_instance.app_server.public_ip # Depends on the EC2 Instance
}

# 14. Output the S3 Bucket Name
output "app_bucket_name" {
  description = "Name of the S3 data bucket"
  value       = aws_s3_bucket.app_data_bucket.bucket # Depends on the S3 Bucket
}
