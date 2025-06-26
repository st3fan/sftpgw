package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	ServerPort         int
	VirtualDir         string
	MaxFileSize        int64
	S3Bucket           string
	S3BucketPrefix     string
	S3Region           string
	RequiredAccountID  string
	ConnectionTimeout  time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	MaxConnections     int
}

func LoadConfig() (*Config, error) {
	config := &Config{
		ServerPort:        2222,
		VirtualDir:        "/uploads",
		MaxFileSize:       1024 * 1024, // 1MB default
		ConnectionTimeout: 30 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		MaxConnections:    100,
	}

	if port := os.Getenv("SFTP_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err != nil {
			return nil, fmt.Errorf("invalid SFTP_PORT: %w", err)
		} else {
			config.ServerPort = p
		}
	}

	if vdir := os.Getenv("VIRTUAL_DIR"); vdir != "" {
		config.VirtualDir = vdir
	}

	// Clean and ensure VirtualDir is an absolute path
	config.VirtualDir = filepath.Clean(config.VirtualDir)
	if !filepath.IsAbs(config.VirtualDir) {
		config.VirtualDir = "/" + config.VirtualDir
	}

	if maxSize := os.Getenv("MAX_FILE_SIZE"); maxSize != "" {
		if size, err := strconv.ParseInt(maxSize, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid MAX_FILE_SIZE: %w", err)
		} else {
			config.MaxFileSize = size
		}
	}

	if bucket := os.Getenv("S3_BUCKET"); bucket != "" {
		config.S3Bucket = bucket
	} else {
		return nil, fmt.Errorf("S3_BUCKET environment variable is required")
	}

	if prefix := os.Getenv("S3_BUCKET_PREFIX"); prefix != "" {
		config.S3BucketPrefix = prefix
	}

	if region := os.Getenv("AWS_REGION"); region != "" {
		config.S3Region = region
	}

	if accountID := os.Getenv("AWS_ACCOUNT_ID"); accountID != "" {
		config.RequiredAccountID = accountID
	} else {
		return nil, fmt.Errorf("AWS_ACCOUNT_ID environment variable is required")
	}

	if timeout := os.Getenv("CONNECTION_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err != nil {
			return nil, fmt.Errorf("invalid CONNECTION_TIMEOUT: %w", err)
		} else {
			config.ConnectionTimeout = t
		}
	}

	if timeout := os.Getenv("READ_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err != nil {
			return nil, fmt.Errorf("invalid read_TIMEOUT: %w", err)
		} else {
			config.ReadTimeout = t
		}
	}

	if timeout := os.Getenv("WRITE_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err != nil {
			return nil, fmt.Errorf("invalid WRITE_TIMEOUT: %w", err)
		} else {
			config.WriteTimeout = t
		}
	}

	if maxConns := os.Getenv("MAX_CONNECTIONS"); maxConns != "" {
		if max, err := strconv.Atoi(maxConns); err != nil {
			return nil, fmt.Errorf("invalid MAX_CONNECTIONS: %w", err)
		} else {
			config.MaxConnections = max
		}
	}

	return config, nil
}