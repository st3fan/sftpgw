package main

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig_RequiredFields(t *testing.T) {
	// Clear all environment variables
	clearEnv()
	
	// Test missing S3_BUCKET
	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error when S3_BUCKET is missing")
	}
	if err.Error() != "S3_BUCKET environment variable is required" {
		t.Errorf("Expected S3_BUCKET error, got: %v", err)
	}
	
	// Set S3_BUCKET but missing AWS_ACCOUNT_ID
	os.Setenv("S3_BUCKET", "test-bucket")
	_, err = LoadConfig()
	if err == nil {
		t.Error("Expected error when AWS_ACCOUNT_ID is missing")
	}
	if err.Error() != "AWS_ACCOUNT_ID environment variable is required" {
		t.Errorf("Expected AWS_ACCOUNT_ID error, got: %v", err)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear all environment variables and set required ones
	clearEnv()
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("AWS_ACCOUNT_ID", "123456789012")
	
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	// Test default values
	if config.ServerPort != 2222 {
		t.Errorf("Expected ServerPort 2222, got %d", config.ServerPort)
	}
	if config.VirtualDir != "/uploads" {
		t.Errorf("Expected VirtualDir '/uploads', got '%s'", config.VirtualDir)
	}
	if config.MaxFileSize != 1024*1024 {
		t.Errorf("Expected MaxFileSize 1048576, got %d", config.MaxFileSize)
	}
	if config.S3Bucket != "test-bucket" {
		t.Errorf("Expected S3Bucket 'test-bucket', got '%s'", config.S3Bucket)
	}
	if config.RequiredAccountID != "123456789012" {
		t.Errorf("Expected RequiredAccountID '123456789012', got '%s'", config.RequiredAccountID)
	}
	if config.ConnectionTimeout != 30*time.Second {
		t.Errorf("Expected ConnectionTimeout 30s, got %v", config.ConnectionTimeout)
	}
	if config.ReadTimeout != 30*time.Second {
		t.Errorf("Expected ReadTimeout 30s, got %v", config.ReadTimeout)
	}
	if config.WriteTimeout != 30*time.Second {
		t.Errorf("Expected WriteTimeout 30s, got %v", config.WriteTimeout)
	}
	if config.MaxConnections != 100 {
		t.Errorf("Expected MaxConnections 100, got %d", config.MaxConnections)
	}
	if config.S3BucketPrefix != "" {
		t.Errorf("Expected empty S3BucketPrefix, got '%s'", config.S3BucketPrefix)
	}
	if config.S3Region != "" {
		t.Errorf("Expected empty S3Region, got '%s'", config.S3Region)
	}
}

func TestLoadConfig_CustomValues(t *testing.T) {
	// Clear all environment variables and set custom values
	clearEnv()
	os.Setenv("S3_BUCKET", "custom-bucket")
	os.Setenv("S3_BUCKET_PREFIX", "uploads/sftp")
	os.Setenv("AWS_REGION", "us-west-2")
	os.Setenv("AWS_ACCOUNT_ID", "987654321098")
	os.Setenv("SFTP_PORT", "2223")
	os.Setenv("VIRTUAL_DIR", "/custom-uploads")
	os.Setenv("MAX_FILE_SIZE", "5242880") // 5MB
	os.Setenv("CONNECTION_TIMEOUT", "60s")
	os.Setenv("READ_TIMEOUT", "45s")
	os.Setenv("WRITE_TIMEOUT", "90s")
	os.Setenv("MAX_CONNECTIONS", "50")
	
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	// Test custom values
	if config.ServerPort != 2223 {
		t.Errorf("Expected ServerPort 2223, got %d", config.ServerPort)
	}
	if config.VirtualDir != "/custom-uploads" {
		t.Errorf("Expected VirtualDir '/custom-uploads', got '%s'", config.VirtualDir)
	}
	if config.MaxFileSize != 5242880 {
		t.Errorf("Expected MaxFileSize 5242880, got %d", config.MaxFileSize)
	}
	if config.S3Bucket != "custom-bucket" {
		t.Errorf("Expected S3Bucket 'custom-bucket', got '%s'", config.S3Bucket)
	}
	if config.S3BucketPrefix != "uploads/sftp" {
		t.Errorf("Expected S3BucketPrefix 'uploads/sftp', got '%s'", config.S3BucketPrefix)
	}
	if config.S3Region != "us-west-2" {
		t.Errorf("Expected S3Region 'us-west-2', got '%s'", config.S3Region)
	}
	if config.RequiredAccountID != "987654321098" {
		t.Errorf("Expected RequiredAccountID '987654321098', got '%s'", config.RequiredAccountID)
	}
	if config.ConnectionTimeout != 60*time.Second {
		t.Errorf("Expected ConnectionTimeout 60s, got %v", config.ConnectionTimeout)
	}
	if config.ReadTimeout != 45*time.Second {
		t.Errorf("Expected ReadTimeout 45s, got %v", config.ReadTimeout)
	}
	if config.WriteTimeout != 90*time.Second {
		t.Errorf("Expected WriteTimeout 90s, got %v", config.WriteTimeout)
	}
	if config.MaxConnections != 50 {
		t.Errorf("Expected MaxConnections 50, got %d", config.MaxConnections)
	}
}

func TestLoadConfig_InvalidValues(t *testing.T) {
	// Test invalid SFTP_PORT
	clearEnv()
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("AWS_ACCOUNT_ID", "123456789012")
	os.Setenv("SFTP_PORT", "invalid")
	
	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error for invalid SFTP_PORT")
	}
	
	// Test invalid MAX_FILE_SIZE
	clearEnv()
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("AWS_ACCOUNT_ID", "123456789012")
	os.Setenv("MAX_FILE_SIZE", "invalid")
	
	_, err = LoadConfig()
	if err == nil {
		t.Error("Expected error for invalid MAX_FILE_SIZE")
	}
	
	// Test invalid CONNECTION_TIMEOUT
	clearEnv()
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("AWS_ACCOUNT_ID", "123456789012")
	os.Setenv("CONNECTION_TIMEOUT", "invalid")
	
	_, err = LoadConfig()
	if err == nil {
		t.Error("Expected error for invalid CONNECTION_TIMEOUT")
	}
	
	// Test invalid READ_TIMEOUT
	clearEnv()
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("AWS_ACCOUNT_ID", "123456789012")
	os.Setenv("READ_TIMEOUT", "invalid")
	
	_, err = LoadConfig()
	if err == nil {
		t.Error("Expected error for invalid READ_TIMEOUT")
	}
	
	// Test invalid WRITE_TIMEOUT
	clearEnv()
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("AWS_ACCOUNT_ID", "123456789012")
	os.Setenv("WRITE_TIMEOUT", "invalid")
	
	_, err = LoadConfig()
	if err == nil {
		t.Error("Expected error for invalid WRITE_TIMEOUT")
	}
	
	// Test invalid MAX_CONNECTIONS
	clearEnv()
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("AWS_ACCOUNT_ID", "123456789012")
	os.Setenv("MAX_CONNECTIONS", "invalid")
	
	_, err = LoadConfig()
	if err == nil {
		t.Error("Expected error for invalid MAX_CONNECTIONS")
	}
}

func TestLoadConfig_EmptyValues(t *testing.T) {
	// Test that empty environment variables are handled correctly
	clearEnv()
	os.Setenv("S3_BUCKET", "test-bucket")
	os.Setenv("AWS_ACCOUNT_ID", "123456789012")
	os.Setenv("SFTP_PORT", "")
	os.Setenv("VIRTUAL_DIR", "")
	os.Setenv("S3_BUCKET_PREFIX", "")
	os.Setenv("AWS_REGION", "")
	
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	// Should use defaults for empty values
	if config.ServerPort != 2222 {
		t.Errorf("Expected default ServerPort 2222, got %d", config.ServerPort)
	}
	if config.VirtualDir != "/uploads" {
		t.Errorf("Expected default VirtualDir '/uploads', got '%s'", config.VirtualDir)
	}
}

// Helper function to clear relevant environment variables
func clearEnv() {
	envVars := []string{
		"SFTP_PORT",
		"VIRTUAL_DIR",
		"MAX_FILE_SIZE",
		"S3_BUCKET",
		"S3_BUCKET_PREFIX",
		"AWS_REGION",
		"AWS_ACCOUNT_ID",
		"CONNECTION_TIMEOUT",
		"READ_TIMEOUT",
		"WRITE_TIMEOUT",
		"MAX_CONNECTIONS",
	}
	
	for _, env := range envVars {
		os.Unsetenv(env)
	}
}