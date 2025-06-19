package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type IntegrationTestSuite struct {
	server       *SFTPServer
	serverAddr   string
	s3Client     *s3.Client
	testBucket   string
	testPrefix   string
	awsRegion    string
	awsAccountID string
	sftpUser     string
	sftpPass     string
	logger       *slog.Logger
	cleanupKeys  []string
}

func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	suite := setupIntegrationTest(t)
	defer suite.cleanup(t)

	t.Run("Authentication", suite.testAuthentication)
	t.Run("FileUpload", suite.testFileUpload)
	t.Run("DirectoryOperations", suite.testDirectoryOperations)
	t.Run("FileOperations", suite.testFileOperations)
	t.Run("ErrorHandling", suite.testErrorHandling)
	t.Run("LargeFileUpload", suite.testLargeFileUpload)
	t.Run("ConcurrentUploads", suite.testConcurrentUploads)
}

func setupIntegrationTest(t *testing.T) *IntegrationTestSuite {
	// Check required environment variables
	awsRegion := os.Getenv("AWS_REGION")
	awsAccountID := os.Getenv("AWS_ACCOUNT_ID")
	testBucket := os.Getenv("S3_BUCKET")
	testPrefix := os.Getenv("S3_BUCKET_PREFIX")
	sftpUser := os.Getenv("SFTP_USER")
	sftpPass := os.Getenv("SFTP_PASS")

	if awsRegion == "" || awsAccountID == "" || testBucket == "" || sftpUser == "" || sftpPass == "" {
		t.Fatal("Required environment variables not set: AWS_REGION, AWS_ACCOUNT_ID, S3_BUCKET, SFTP_USER, SFTP_PASS")
	}

	if testPrefix == "" {
		testPrefix = "integration-test"
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Setup S3 client for test verification
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(awsRegion),
	)
	if err != nil {
		t.Fatalf("Failed to load AWS config: %v", err)
	}
	s3Client := s3.NewFromConfig(cfg)

	// Setup test server configuration
	config := &Config{
		ServerPort:        0, // Use random available port
		VirtualDir:        "/uploads",
		MaxFileSize:       10 * 1024 * 1024, // 10MB for testing
		S3Bucket:          testBucket,
		S3BucketPrefix:    testPrefix,
		S3Region:          awsRegion,
		RequiredAccountID: awsAccountID,
		ConnectionTimeout: 30 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		MaxConnections:    10,
	}

	// Create and start server
	server := &SFTPServer{
		config: config,
		logger: logger,
	}

	if err := server.setupSSHConfig(); err != nil {
		t.Fatalf("Failed to setup SSH config: %v", err)
	}

	server.uploader = NewS3Uploader(config.S3Bucket, config.S3BucketPrefix, config.S3Region, logger)
	server.handler = NewSFTPHandler(config, server.uploader, logger)
	server.auth = NewAuthenticator(config.RequiredAccountID, config.S3Region, logger)
	server.sshConfig.PasswordCallback = server.auth.Authenticate

	// Start server on random port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	server.listener = listener

	serverAddr := listener.Addr().String()

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		server.acceptConnections(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	suite := &IntegrationTestSuite{
		server:       server,
		serverAddr:   serverAddr,
		s3Client:     s3Client,
		testBucket:   testBucket,
		testPrefix:   testPrefix,
		awsRegion:    awsRegion,
		awsAccountID: awsAccountID,
		sftpUser:     sftpUser,
		sftpPass:     sftpPass,
		logger:       logger,
		cleanupKeys:  make([]string, 0),
	}

	return suite
}

func (suite *IntegrationTestSuite) cleanup(t *testing.T) {
	// Close server
	if suite.server.listener != nil {
		suite.server.listener.Close()
	}

	// Clean up S3 objects created during testing
	for _, key := range suite.cleanupKeys {
		suite.deleteS3Object(t, key)
	}
}

func (suite *IntegrationTestSuite) deleteS3Object(t *testing.T, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := suite.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(suite.testBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Logf("Warning: Failed to cleanup S3 object %s: %v", key, err)
	}
}

func (suite *IntegrationTestSuite) connectSFTP(t *testing.T) (*sftp.Client, func()) {
	config := &ssh.ClientConfig{
		User: suite.sftpUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(suite.sftpPass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	conn, err := ssh.Dial("tcp", suite.serverAddr, config)
	if err != nil {
		t.Fatalf("Failed to connect via SSH: %v", err)
	}

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		t.Fatalf("Failed to create SFTP client: %v", err)
	}

	cleanup := func() {
		sftpClient.Close()
		conn.Close()
	}

	return sftpClient, cleanup
}

func (suite *IntegrationTestSuite) testAuthentication(t *testing.T) {
	t.Run("ValidCredentials", func(t *testing.T) {
		client, cleanup := suite.connectSFTP(t)
		defer cleanup()

		// Just test that we can connect successfully
		if client == nil {
			t.Fatal("Expected successful connection with valid credentials")
		}
	})

	t.Run("InvalidCredentials", func(t *testing.T) {
		config := &ssh.ClientConfig{
			User: "invalid-user",
			Auth: []ssh.AuthMethod{
				ssh.Password("invalid-password"),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
		}

		conn, err := ssh.Dial("tcp", suite.serverAddr, config)
		if err == nil {
			conn.Close()
			t.Fatal("Expected authentication to fail with invalid credentials")
		}
	})
}

func (suite *IntegrationTestSuite) testFileUpload(t *testing.T) {
	client, cleanup := suite.connectSFTP(t)
	defer cleanup()

	t.Run("SimpleFileUpload", func(t *testing.T) {
		testData := []byte("Hello, SFTP integration test!")
		testFilename := fmt.Sprintf("test-file-%d.txt", time.Now().Unix())
		testPath := "/uploads/" + testFilename

		// Upload file
		file, err := client.Create(testPath)
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		_, err = file.Write(testData)
		if err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		err = file.Close()
		if err != nil {
			t.Fatalf("Failed to close file: %v", err)
		}

		// Verify file exists in S3
		s3Key := suite.generateExpectedS3Key(testFilename)
		suite.cleanupKeys = append(suite.cleanupKeys, s3Key)
		suite.verifyS3Object(t, s3Key, testData)
	})

	t.Run("BinaryFileUpload", func(t *testing.T) {
		// Create random binary data
		testData := make([]byte, 1024)
		_, err := rand.Read(testData)
		if err != nil {
			t.Fatalf("Failed to generate test data: %v", err)
		}

		testFilename := fmt.Sprintf("binary-test-%d.bin", time.Now().Unix())
		testPath := "/uploads/" + testFilename

		// Upload file
		file, err := client.Create(testPath)
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		_, err = file.Write(testData)
		if err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		err = file.Close()
		if err != nil {
			t.Fatalf("Failed to close file: %v", err)
		}

		// Verify file exists in S3
		s3Key := suite.generateExpectedS3Key(testFilename)
		suite.cleanupKeys = append(suite.cleanupKeys, s3Key)
		suite.verifyS3Object(t, s3Key, testData)
	})
}

func (suite *IntegrationTestSuite) testDirectoryOperations(t *testing.T) {
	client, cleanup := suite.connectSFTP(t)
	defer cleanup()

	t.Run("MkdirAllowed", func(t *testing.T) {
		// Mkdir should succeed (but be virtual)
		err := client.Mkdir("/uploads/testdir")
		if err != nil {
			t.Fatalf("Mkdir should be allowed: %v", err)
		}
	})

	t.Run("ListingDenied", func(t *testing.T) {
		// Directory listing should be denied
		_, err := client.ReadDir("/uploads")
		if err == nil {
			t.Fatal("Directory listing should be denied")
		}
	})

	t.Run("RmdirDenied", func(t *testing.T) {
		// Rmdir should be denied
		err := client.RemoveDirectory("/uploads/testdir")
		if err == nil {
			t.Fatal("Rmdir should be denied")
		}
	})
}

func (suite *IntegrationTestSuite) testFileOperations(t *testing.T) {
	client, cleanup := suite.connectSFTP(t)
	defer cleanup()

	// First upload a test file
	testData := []byte("test file for operations")
	testFilename := fmt.Sprintf("ops-test-%d.txt", time.Now().Unix())
	testPath := "/uploads/" + testFilename

	file, err := client.Create(testPath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	file.Write(testData)
	file.Close()

	s3Key := suite.generateExpectedS3Key(testFilename)
	suite.cleanupKeys = append(suite.cleanupKeys, s3Key)

	t.Run("ReadDenied", func(t *testing.T) {
		// File reading should be denied
		_, err := client.Open(testPath)
		if err == nil {
			t.Fatal("File reading should be denied")
		}
	})

	t.Run("StatDenied", func(t *testing.T) {
		// File stat should be denied
		_, err := client.Stat(testPath)
		if err == nil {
			t.Fatal("File stat should be denied")
		}
	})

	t.Run("RemoveDenied", func(t *testing.T) {
		// File removal should be denied
		err := client.Remove(testPath)
		if err == nil {
			t.Fatal("File removal should be denied")
		}
	})

	t.Run("RenameDenied", func(t *testing.T) {
		// File rename should be denied
		err := client.Rename(testPath, "/uploads/renamed.txt")
		if err == nil {
			t.Fatal("File rename should be denied")
		}
	})
}

func (suite *IntegrationTestSuite) testErrorHandling(t *testing.T) {
	client, cleanup := suite.connectSFTP(t)
	defer cleanup()

	t.Run("PathOutsideVirtualDir", func(t *testing.T) {
		// Try to upload outside virtual directory
		_, err := client.Create("/etc/passwd")
		if err == nil {
			t.Fatal("Upload outside virtual directory should be denied")
		}
	})

	t.Run("PathTraversalAttack", func(t *testing.T) {
		// Try path traversal attack
		_, err := client.Create("/uploads/../../../etc/passwd")
		if err == nil {
			t.Fatal("Path traversal attack should be denied")
		}
	})

	t.Run("EmptyFilename", func(t *testing.T) {
		// Try to create file with empty name
		_, err := client.Create("/uploads/")
		if err == nil {
			t.Fatal("Empty filename should be denied")
		}
	})
}

func (suite *IntegrationTestSuite) testLargeFileUpload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	client, cleanup := suite.connectSFTP(t)
	defer cleanup()

	// Create 1MB test file
	testData := make([]byte, 1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	testFilename := fmt.Sprintf("large-test-%d.bin", time.Now().Unix())
	testPath := "/uploads/" + testFilename

	file, err := client.Create(testPath)
	if err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	// Write in chunks to simulate real-world usage
	chunkSize := 8192
	for i := 0; i < len(testData); i += chunkSize {
		end := i + chunkSize
		if end > len(testData) {
			end = len(testData)
		}

		_, err = file.Write(testData[i:end])
		if err != nil {
			t.Fatalf("Failed to write chunk: %v", err)
		}
	}

	err = file.Close()
	if err != nil {
		t.Fatalf("Failed to close large file: %v", err)
	}

	// Verify file exists in S3
	s3Key := suite.generateExpectedS3Key(testFilename)
	suite.cleanupKeys = append(suite.cleanupKeys, s3Key)
	suite.verifyS3Object(t, s3Key, testData)
}

func (suite *IntegrationTestSuite) testConcurrentUploads(t *testing.T) {
	const numUploads = 5

	// Create multiple SFTP clients for concurrent uploads
	clients := make([]*sftp.Client, numUploads)
	cleanups := make([]func(), numUploads)

	for i := 0; i < numUploads; i++ {
		clients[i], cleanups[i] = suite.connectSFTP(t)
		defer cleanups[i]()
	}

	// Channels for synchronization
	results := make(chan error, numUploads)

	// Start concurrent uploads
	for i := 0; i < numUploads; i++ {
		go func(clientIndex int) {
			testData := []byte(fmt.Sprintf("Concurrent upload test data %d", clientIndex))
			testFilename := fmt.Sprintf("concurrent-test-%d-%d.txt", clientIndex, time.Now().Unix())
			testPath := "/uploads/" + testFilename

			file, err := clients[clientIndex].Create(testPath)
			if err != nil {
				results <- fmt.Errorf("client %d failed to create file: %v", clientIndex, err)
				return
			}

			_, err = file.Write(testData)
			if err != nil {
				results <- fmt.Errorf("client %d failed to write file: %v", clientIndex, err)
				return
			}

			err = file.Close()
			if err != nil {
				results <- fmt.Errorf("client %d failed to close file: %v", clientIndex, err)
				return
			}

			// Verify S3 upload
			s3Key := suite.generateExpectedS3Key(testFilename)
			suite.cleanupKeys = append(suite.cleanupKeys, s3Key)

			// Give S3 upload time to complete
			time.Sleep(2 * time.Second)

			if !suite.s3ObjectExists(t, s3Key) {
				results <- fmt.Errorf("client %d: S3 object not found: %s", clientIndex, s3Key)
				return
			}

			results <- nil
		}(i)
	}

	// Wait for all uploads to complete
	for i := 0; i < numUploads; i++ {
		err := <-results
		if err != nil {
			t.Errorf("Concurrent upload failed: %v", err)
		}
	}
}

func (suite *IntegrationTestSuite) generateExpectedS3Key(filename string) string {
	timestamp := time.Now().UTC().Format("2006-01-02")
	sanitizedFilename := strings.ReplaceAll(filename, " ", "_")
	sanitizedFilename = strings.ReplaceAll(sanitizedFilename, "..", "_")

	if suite.testPrefix != "" {
		return fmt.Sprintf("%s/%s/%s", suite.testPrefix, timestamp, sanitizedFilename)
	}
	return fmt.Sprintf("%s/%s", timestamp, sanitizedFilename)
}

func (suite *IntegrationTestSuite) verifyS3Object(t *testing.T, key string, expectedData []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Give S3 some time to complete the upload
	time.Sleep(2 * time.Second)

	result, err := suite.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(suite.testBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Fatalf("Failed to get S3 object %s: %v", key, err)
	}
	defer result.Body.Close()

	actualData, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("Failed to read S3 object data: %v", err)
	}

	if !bytes.Equal(actualData, expectedData) {
		t.Fatalf("S3 object data mismatch. Expected %d bytes, got %d bytes", len(expectedData), len(actualData))
	}

	// Verify metadata
	if result.Metadata == nil {
		t.Fatal("Expected metadata on S3 object")
	}

	if result.Metadata["access-key-id"] != suite.sftpUser {
		t.Errorf("Expected access-key-id metadata to be %s, got %s", suite.sftpUser, result.Metadata["access-key-id"])
	}

	if result.Metadata["original-path"] == "" {
		t.Error("Expected original-path metadata to be set")
	}
}

func (suite *IntegrationTestSuite) s3ObjectExists(t *testing.T, key string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := suite.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(suite.testBucket),
		Key:    aws.String(key),
	})

	return err == nil
}
