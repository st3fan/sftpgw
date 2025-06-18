package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestS3Uploader_generateS3Key(t *testing.T) {
	// Mock time for consistent testing
	now := time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC)
	
	tests := []struct {
		name         string
		uploader     *S3Uploader
		filePath     string
		expectedDate string
		expectPrefix bool
	}{
		{
			name: "simple filename without prefix",
			uploader: &S3Uploader{
				bucket:       "test-bucket",
				bucketPrefix: "",
			},
			filePath:     "/uploads/test.txt",
			expectedDate: "2023-12-25",
			expectPrefix: false,
		},
		{
			name: "simple filename with prefix", 
			uploader: &S3Uploader{
				bucket:       "test-bucket",
				bucketPrefix: "sftp-uploads",
			},
			filePath:     "/uploads/test.txt",
			expectedDate: "2023-12-25",
			expectPrefix: true,
		},
		{
			name: "filename with spaces",
			uploader: &S3Uploader{
				bucket:       "test-bucket", 
				bucketPrefix: "",
			},
			filePath:     "/uploads/my file.txt",
			expectedDate: "2023-12-25",
			expectPrefix: false,
		},
		{
			name: "filename with path traversal attempt",
			uploader: &S3Uploader{
				bucket:       "test-bucket",
				bucketPrefix: "",
			},
			filePath:     "/uploads/../../../etc/passwd",
			expectedDate: "2023-12-25",
			expectPrefix: false,
		},
		{
			name: "empty filename",
			uploader: &S3Uploader{
				bucket:       "test-bucket",
				bucketPrefix: "",
			},
			filePath:     "/uploads/",
			expectedDate: "2023-12-25",
			expectPrefix: false,
		},
		{
			name: "root path",
			uploader: &S3Uploader{
				bucket:       "test-bucket",
				bucketPrefix: "",
			},
			filePath:     "/",
			expectedDate: "2023-12-25",
			expectPrefix: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set custom time function for consistent testing
			tt.uploader.timeFunc = func() time.Time { return now }
			
			result := tt.uploader.generateS3Key(tt.filePath)
			
			// Check if result contains expected date
			if !strings.Contains(result, tt.expectedDate) {
				t.Errorf("generateS3Key() = %q, expected to contain date %q", result, tt.expectedDate)
			}
			
			// Check prefix handling
			if tt.expectPrefix {
				if !strings.HasPrefix(result, tt.uploader.bucketPrefix+"/") {
					t.Errorf("generateS3Key() = %q, expected to start with prefix %q", result, tt.uploader.bucketPrefix)
				}
			} else {
				// When no prefix is expected, the result should start with the date
				if !strings.HasPrefix(result, tt.expectedDate+"/") {
					t.Errorf("generateS3Key() = %q, expected to start with date %q when no prefix", result, tt.expectedDate)
				}
			}
			
			// Check sanitization of spaces
			if strings.Contains(tt.filePath, " ") && strings.Contains(result, " ") {
				t.Errorf("generateS3Key() = %q, expected spaces to be replaced with underscores", result)
			}
			
			// Check sanitization of .. 
			if strings.Contains(tt.filePath, "..") && strings.Contains(result, "..") {
				t.Errorf("generateS3Key() = %q, expected .. to be replaced with underscores", result)
			}
			
			// Check that only specific edge cases become "unknown"
			filename := filepath.Base(tt.filePath)
			if (filename == "" || filename == "." || filename == "/") && !strings.Contains(result, "unknown") {
				t.Errorf("generateS3Key() = %q, expected invalid filename to become 'unknown'", result)
			}
		})
	}
}

func TestS3Uploader_generateS3Key_Sanitization(t *testing.T) {
	uploader := &S3Uploader{
		bucket:       "test-bucket",
		bucketPrefix: "",
		logger:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
		timeFunc:     time.Now,
	}
	
	tests := []struct {
		name         string
		filePath     string
		expectInKey  string
		notExpectInKey string
	}{
		{
			name:           "spaces replaced with underscores",
			filePath:       "/uploads/my file name.txt",
			expectInKey:    "my_file_name.txt",
			notExpectInKey: " ",
		},
		{
			name:           "path traversal replaced",
			filePath:       "/uploads/my..file.txt", 
			expectInKey:    "my_file.txt",
			notExpectInKey: "..",
		},
		{
			name:           "complex filename sanitization",
			filePath:       "/uploads/my file..name with spaces.txt",
			expectInKey:    "my_file_name_with_spaces.txt",
			notExpectInKey: " ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := uploader.generateS3Key(tt.filePath)
			
			if !strings.Contains(result, tt.expectInKey) {
				t.Errorf("generateS3Key() = %q, expected to contain %q", result, tt.expectInKey)
			}
			
			if strings.Contains(result, tt.notExpectInKey) {
				t.Errorf("generateS3Key() = %q, expected NOT to contain %q", result, tt.notExpectInKey)
			}
		})
	}
}

func TestS3Uploader_generateS3Key_EdgeCases(t *testing.T) {
	uploader := &S3Uploader{
		bucket:       "test-bucket",
		bucketPrefix: "uploads/sftp",
		logger:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
		timeFunc:     time.Now,
	}

	edgeCases := []string{
		"",
		".",
		"/",
		"/uploads/.",
	}

	for _, filePath := range edgeCases {
		t.Run("edge_case_"+filePath, func(t *testing.T) {
			result := uploader.generateS3Key(filePath)
			
			// All edge cases should result in "unknown" filename
			if !strings.Contains(result, "unknown") {
				t.Errorf("generateS3Key(%q) = %q, expected to contain 'unknown' for edge case", filePath, result)
			}
			
			// Should still have proper structure with prefix and date
			expectedParts := []string{uploader.bucketPrefix, "unknown"}
			for _, part := range expectedParts {
				if !strings.Contains(result, part) {
					t.Errorf("generateS3Key(%q) = %q, expected to contain %q", filePath, result, part)
				}
			}
		})
	}
	
	// Test cases that do NOT become "unknown"
	nonUnknownCases := []struct{
		input string
		expectedFilename string
	}{
		{"/uploads/", "uploads"},
		{"/uploads/..", "_"},
		{"/uploads/...", "_."},
	}
	
	for _, tc := range nonUnknownCases {
		t.Run("non_unknown_case_"+tc.input, func(t *testing.T) {
			result := uploader.generateS3Key(tc.input)
			
			// Should contain the expected filename, not "unknown"
			if !strings.Contains(result, tc.expectedFilename) {
				t.Errorf("generateS3Key(%q) = %q, expected to contain filename %q", tc.input, result, tc.expectedFilename)
			}
			
			if strings.Contains(result, "unknown") {
				t.Errorf("generateS3Key(%q) = %q, expected NOT to contain 'unknown'", tc.input, result)
			}
		})
	}
}