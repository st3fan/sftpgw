# SFTP Gateway with S3 Backend

A write-only SFTP server that uses AWS S3 as the storage backend and AWS IAM credentials for authentication.

## Features

- **Write-only SFTP server**: Accepts file uploads but rejects read/list operations
- **AWS IAM Authentication**: Uses AWS Access Key ID and Secret Access Key as SFTP credentials
- **S3 Storage Backend**: Automatically uploads files to a configured S3 bucket
- **Account Validation**: Validates that credentials belong to a specific AWS Account ID
- **File Size Limits**: Configurable maximum file size (default: 1MB)
- **Structured Logging**: Comprehensive logging using Go's `log/slog` package
- **Security**: Path validation, directory traversal prevention, and connection limits

## Configuration

The server is configured via environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SFTP_PORT` | No | `2222` | SFTP server port |
| `VIRTUAL_DIR` | No | `/uploads` | Virtual directory path for file uploads |
| `MAX_FILE_SIZE` | No | `1048576` (1MB) | Maximum file size in bytes |
| `S3_BUCKET` | **Yes** | - | S3 bucket name for file storage |
| `S3_BUCKET_PREFIX` | No | - | Optional prefix for S3 object keys |
| `AWS_REGION` | No | - | AWS region for S3 bucket (auto-detected if not specified) |
| `AWS_ACCOUNT_ID` | **Yes** | - | Required AWS Account ID for credential validation |
| `CONNECTION_TIMEOUT` | No | `30s` | Connection timeout duration |
| `READ_TIMEOUT` | No | `30s` | Read operation timeout |
| `WRITE_TIMEOUT` | No | `30s` | Write operation timeout |
| `MAX_CONNECTIONS` | No | `100` | Maximum concurrent connections |

## Setup

### Prerequisites

- Go 1.21 or later
- AWS account with appropriate permissions
- S3 bucket for file storage

### Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/st3fan/sftpgw.git
   cd sftpgw
   ```

2. Build the application:
   ```bash
   go build -o sftpgw .
   ```

### AWS Permissions

The IAM user used for authentication needs the following permissions. Use this complete IAM policy for least privilege access:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "STSCredentialValidation",
      "Effect": "Allow",
      "Action": [
        "sts:GetCallerIdentity"
      ],
      "Resource": "*"
    },
    {
      "Sid": "S3FileUpload",
      "Effect": "Allow",
      "Action": [
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::your-bucket-name/*"
    }
  ]
}
```

**Policy Explanation:**
- **STS permissions**: `sts:GetCallerIdentity` allows the server to validate credentials and retrieve the AWS Account ID
- **S3 permissions**: `s3:PutObject` allows uploading files to the specified S3 bucket
- **Least privilege**: Only the minimum required permissions are granted - no read, list, or delete capabilities

## Usage

### Starting the Server

1. Set required environment variables:
   ```bash
   export S3_BUCKET=your-s3-bucket-name
   export AWS_ACCOUNT_ID=123456789012
   ```

2. Optional: Set additional configuration:
   ```bash
   export SFTP_PORT=2222
   export MAX_FILE_SIZE=10485760  # 10MB
   export VIRTUAL_DIR=/uploads
   export S3_BUCKET_PREFIX=uploads  # Optional prefix for S3 keys
   export AWS_REGION=us-west-2  # Set if bucket is in specific region
   ```

3. Start the server:
   ```bash
   ./sftpgw
   ```

### Connecting via SFTP

Use any SFTP client with your AWS credentials:

```bash
sftp -P 2222 AKIAIOSFODNN7EXAMPLE@localhost
# Enter your AWS Secret Access Key when prompted for password
```

### Example with `sftp` command:

```bash
$ sftp -P 2222 AKIAIOSFODNN7EXAMPLE@localhost
AKIAIOSFODNN7EXAMPLE@localhost's password: [enter secret access key]
Connected to localhost.
sftp> cd /uploads
sftp> put myfile.txt
Uploading myfile.txt to /uploads/myfile.txt
myfile.txt                    100%  1024     1.0KB/s   00:00
sftp> quit
```

## File Organization in S3

Files are organized in S3 with the following structure:

```
s3://your-bucket/
├── uploads/2024-01-15/myfile.txt
├── uploads/2024-01-15/another-file.pdf
└── uploads/2024-01-16/document.docx
```

**Without prefix:**
- Path structure: `YYYY-MM-DD/FILENAME`

**With S3_BUCKET_PREFIX set to "uploads":**
- Path structure: `S3_BUCKET_PREFIX/YYYY-MM-DD/FILENAME`

## Logging

The server provides structured JSON logging with the following information:

- **Authentication attempts**: Success/failure with client IP and Access Key ID
- **File uploads**: File details, client IP, and upload status
- **Errors**: Detailed error information with context
- **Connection events**: SSH and SFTP session lifecycle

Example log entry:
```json
{
  "time": "2024-01-15T14:30:45Z",
  "level": "INFO",
  "msg": "authentication successful",
  "auth": {
    "remote_ip": "192.168.1.100",
    "access_key_id": "AKIAIOSFODNN7EXAMPLE",
    "account_id": "123456789012",
    "user_id": "AIDACKCEVSQ6C2EXAMPLE",
    "arn": "arn:aws:iam::123456789012:user/example-user"
  }
}
```

## Security Considerations

- **No sensitive data in logs**: AWS secret keys are never logged
- **Path validation**: Prevents directory traversal attacks
- **Connection limits**: Configurable maximum concurrent connections
- **Timeouts**: Prevents resource exhaustion from hanging connections
- **Account validation**: Ensures only specified AWS account credentials are accepted
- **Write-only**: Read and list operations are explicitly blocked

## Supported SFTP Operations

| Operation | Supported | Notes |
|-----------|-----------|-------|
| `put` (upload) | ✅ | Files uploaded to S3 |
| `get` (download) | ❌ | Returns permission denied |
| `ls` (list) | ❌ | Returns permission denied |
| `mkdir` | ✅* | Virtual operation (no actual directory created) |
| `rmdir` | ❌ | Returns permission denied |
| `rm` (delete) | ❌ | Returns permission denied |
| `rename` | ❌ | Returns permission denied |

*mkdir is allowed for client compatibility but doesn't create actual directories.

## Error Handling

The server handles various error conditions gracefully:

- **Invalid credentials**: STS validation failure
- **Wrong AWS account**: Account ID mismatch
- **File too large**: Exceeds configured size limit
- **S3 upload failure**: Network or permission issues
- **Path traversal attempts**: Blocked with error

## Development

### Running Tests

```bash
go test ./...
```

### Building for Production

```bash
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o sftpgw .
```

## Troubleshooting

### Common Issues

1. **Authentication failures**:
   - Verify AWS credentials are correct
   - Check that the Account ID matches the configured value
   - Ensure STS permissions are granted

2. **S3 upload failures**:
   - Verify S3 bucket exists and is accessible
   - Check S3 permissions for the IAM user
   - Ensure bucket is in the correct region

3. **Connection issues**:
   - Check firewall settings for the configured port
   - Verify SFTP client supports password authentication

### Debug Logging

To enable debug logging, modify the logger level in `main.go`:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.
