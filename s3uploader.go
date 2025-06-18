package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Uploader struct {
	bucket       string
	bucketPrefix string
	region       string
	logger       *slog.Logger
	timeFunc     func() time.Time
}

func NewS3Uploader(bucket, bucketPrefix, region string, logger *slog.Logger) *S3Uploader {
	return &S3Uploader{
		bucket:       bucket,
		bucketPrefix: bucketPrefix,
		region:       region,
		logger:       logger,
		timeFunc:     time.Now,
	}
}

func (u *S3Uploader) UploadFile(ctx context.Context, accessKeyID, secretAccessKey, clientIP, filePath string, data []byte) error {
	logCtx := slog.Group("s3_upload",
		"remote_ip", clientIP,
		"access_key_id", accessKeyID,
		"file_path", filePath,
		"file_size", len(data),
		"bucket", u.bucket,
	)

	u.logger.Info("starting S3 upload", logCtx)

	configOptions := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyID,
			secretAccessKey,
			"",
		)),
	}

	if u.region != "" {
		configOptions = append(configOptions, config.WithRegion(u.region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		u.logger.Error("failed to load AWS config for upload", logCtx, slog.String("error", err.Error()))
		return fmt.Errorf("failed to configure AWS client: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg)

	key := u.generateS3Key(filePath)

	uploadCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, err = s3Client.PutObject(uploadCtx, &s3.PutObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
		Metadata: map[string]string{
			"client-ip":       clientIP,
			"access-key-id":   accessKeyID,
			"upload-time":     u.timeFunc().UTC().Format(time.RFC3339),
			"original-path":   filePath,
		},
	})

	if err != nil {
		u.logger.Error("S3 upload failed", logCtx, 
			slog.String("s3_key", key),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	u.logger.Info("S3 upload successful", logCtx, slog.String("s3_key", key))
	return nil
}

func (u *S3Uploader) generateS3Key(filePath string) string {
	timestamp := u.timeFunc().UTC().Format("2006-01-02")
	
	filename := filepath.Base(filePath)
	if filename == "" || filename == "." || filename == "/" {
		filename = "unknown"
	}
	
	sanitizedFilename := strings.ReplaceAll(filename, " ", "_")
	sanitizedFilename = strings.ReplaceAll(sanitizedFilename, "..", "_")
	
	if u.bucketPrefix != "" {
		return fmt.Sprintf("%s/%s/%s", u.bucketPrefix, timestamp, sanitizedFilename)
	}
	return fmt.Sprintf("%s/%s", timestamp, sanitizedFilename)
}