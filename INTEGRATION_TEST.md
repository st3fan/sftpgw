# Integration Tests

This document describes how to run the comprehensive integration tests for the SFTP Gateway.

## Overview

The integration tests verify end-to-end functionality by:
1. Starting a real SFTP server instance
2. Connecting via SFTP client with AWS credentials
3. Testing file upload operations
4. Verifying files are properly stored in S3
5. Testing security controls and error handling

## Prerequisites

### Required Environment Variables

Set the following environment variables before running integration tests:

```bash
export AWS_REGION="us-east-1"              # Your AWS region
export AWS_ACCOUNT_ID="123456789012"       # Your AWS account ID
export S3_BUCKET="your-test-bucket"        # S3 bucket for test uploads
export S3_BUCKET_PREFIX="integration-test" # Prefix for test objects (optional)
export SFTP_USER="AKIAIOSFODNN7EXAMPLE"    # AWS Access Key ID for testing
export SFTP_PASS="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"  # AWS Secret Access Key
```

### AWS Permissions

The test AWS credentials need the following permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "sts:GetCallerIdentity"
            ],
            "Resource": "*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "s3:PutObject",
                "s3:GetObject",
                "s3:HeadObject",
                "s3:DeleteObject"
            ],
            "Resource": "arn:aws:s3:::your-test-bucket/*"
        }
    ]
}
```

### S3 Bucket Setup

1. Create an S3 bucket for testing
2. Ensure the test AWS credentials have appropriate permissions
3. The bucket should be in the same region as specified in `AWS_REGION`

## Running the Tests

### Run All Integration Tests

```bash
go test -v -run TestIntegration
```

### Run Specific Test Categories

```bash
# Test only authentication
go test -v -run TestIntegration/Authentication

# Test only file uploads
go test -v -run TestIntegration/FileUpload

# Test only error handling
go test -v -run TestIntegration/ErrorHandling
```

### Skip Long-Running Tests

```bash
# Skip large file and concurrent upload tests
go test -v -short -run TestIntegration
```

### Run with Race Detection

```bash
go test -v -race -run TestIntegration
```

## Test Categories

### 1. Authentication Tests
- **ValidCredentials**: Verifies successful connection with valid AWS credentials
- **InvalidCredentials**: Verifies connection rejection with invalid credentials

### 2. File Upload Tests
- **SimpleFileUpload**: Tests basic text file upload and S3 verification
- **BinaryFileUpload**: Tests binary data upload with random data

### 3. Directory Operations Tests
- **MkdirAllowed**: Verifies mkdir operations are accepted (but virtual)
- **ListingDenied**: Confirms directory listing is properly denied
- **RmdirDenied**: Confirms directory removal is properly denied

### 4. File Operations Tests
- **ReadDenied**: Confirms file reading is properly denied
- **StatDenied**: Confirms file stat operations are properly denied
- **RemoveDenied**: Confirms file removal is properly denied
- **RenameDenied**: Confirms file rename operations are properly denied

### 5. Error Handling Tests
- **PathOutsideVirtualDir**: Tests security controls for path traversal
- **PathTraversalAttack**: Tests protection against directory traversal attacks
- **EmptyFilename**: Tests handling of invalid filenames

### 6. Performance Tests
- **LargeFileUpload**: Tests uploading a 1MB file (skipped in short mode)
- **ConcurrentUploads**: Tests multiple simultaneous uploads

## Test Verification

The integration tests verify:

1. **SFTP Server Functionality**:
   - Server starts and accepts connections
   - Authentication works correctly
   - File operations behave as expected

2. **S3 Integration**:
   - Files are uploaded to correct S3 locations
   - File content matches uploaded data
   - Proper metadata is attached to S3 objects

3. **Security Controls**:
   - Write-only access is enforced
   - Path traversal attacks are blocked
   - Operations outside virtual directory are denied

4. **Error Handling**:
   - Invalid operations return appropriate errors
   - Server remains stable under error conditions

## Cleanup

The tests automatically clean up S3 objects created during testing. If tests are interrupted, you may need to manually clean up objects in your test bucket with the prefix specified in `S3_BUCKET_PREFIX`.

## Troubleshooting

### Common Issues

1. **Permission Denied**: Check AWS credentials and S3 bucket permissions
2. **Connection Timeout**: Verify AWS region and network connectivity
3. **S3 Object Not Found**: Allow extra time for S3 eventual consistency
4. **Authentication Failed**: Verify AWS account ID matches the configured account

### Debug Logging

To enable debug logging during tests:

```bash
export SFTPGW_LOG_LEVEL=debug
go test -v -run TestIntegration
```

### Manual Testing

You can also test manually using any SFTP client:

```bash
# Using sftp command line client
sftp -P 2222 AKIAIOSFODNN7EXAMPLE@localhost
# Enter AWS Secret Access Key as password when prompted

# Upload a file
put /path/to/local/file /uploads/test.txt
```

## Performance Considerations

- Large file tests upload 1MB files and may take longer
- Concurrent upload tests create multiple connections simultaneously
- S3 eventual consistency may require retries for verification
- Tests include appropriate timeouts and cleanup procedures