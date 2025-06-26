package main

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestSFTPHandler_isPathAllowed(t *testing.T) {
	config := &Config{
		VirtualDir: "/uploads",
	}
	
	handler := NewSFTPHandler(config, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "valid path in virtual directory",
			path:     "/uploads/file.txt",
			expected: true,
		},
		{
			name:     "valid nested path",
			path:     "/uploads/subfolder/file.txt",
			expected: true,
		},
		{
			name:     "exact virtual directory path",
			path:     "/uploads",
			expected: true,
		},
		{
			name:     "path traversal attempt with ..",
			path:     "/uploads/../etc/passwd",
			expected: false,
		},
		{
			name:     "path outside virtual directory",
			path:     "/etc/passwd",
			expected: false,
		},
		{
			name:     "relative path traversal",
			path:     "/uploads/../../etc/passwd",
			expected: false,
		},
		{
			name:     "root path",
			path:     "/",
			expected: false,
		},
		{
			name:     "empty path",
			path:     "",
			expected: false,
		},
		{
			name:     "path with .. in filename (should be allowed)",
			path:     "/uploads/my..file.txt",
			expected: true,
		},
		{
			name:     "path starting with uploads but different directory",
			path:     "/uploads_other/file.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isPathAllowed(tt.path)
			if result != tt.expected {
				t.Errorf("isPathAllowed(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestSFTPHandler_isPathAllowed_CustomVirtualDir(t *testing.T) {
	config := &Config{
		VirtualDir: "/custom/upload/path",
	}
	
	handler := NewSFTPHandler(config, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "valid path in custom virtual directory",
			path:     "/custom/upload/path/file.txt",
			expected: true,
		},
		{
			name:     "path outside custom virtual directory",
			path:     "/custom/upload/other/file.txt",
			expected: false,
		},
		{
			name:     "uploads path should not work with custom dir",
			path:     "/uploads/file.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isPathAllowed(tt.path)
			if result != tt.expected {
				t.Errorf("isPathAllowed(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestFileWriter_WriteAt(t *testing.T) {
	config := &Config{
		MaxFileSize: 1024, // 1KB limit for testing
	}
	
	handler := NewSFTPHandler(config, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	
	upload := &FileUpload{
		data:      make([]byte, 0, config.MaxFileSize),
		path:      "/uploads/test.txt",
		clientIP:  "127.0.0.1",
		accessKey: "test-key",
		secretKey: "test-secret",
	}

	writer := &FileWriter{
		upload:  upload,
		handler: handler,
		logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	tests := []struct {
		name        string
		data        []byte
		offset      int64
		expectError bool
		errorMsg    string
	}{
		{
			name:        "write within size limit",
			data:        []byte("hello world"),
			offset:      0,
			expectError: false,
		},
		{
			name:        "write at offset",  
			data:        []byte("test"),
			offset:      100,
			expectError: false,
		},
		{
			name:        "write exceeding size limit",
			data:        make([]byte, 2048), // 2KB, exceeds 1KB limit
			offset:      0,
			expectError: true,
			errorMsg:    "file too large",
		},
		{
			name:        "write that would exceed limit with offset",
			data:        make([]byte, 500),
			offset:      600, // 500 + 600 = 1100 > 1024
			expectError: true,
			errorMsg:    "file too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the upload data for each test
			upload.data = make([]byte, 0, config.MaxFileSize)
			writer.closed = false
			
			n, err := writer.WriteAt(tt.data, tt.offset)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("WriteAt() expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("WriteAt() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("WriteAt() unexpected error: %v", err)
				}
				if n != len(tt.data) {
					t.Errorf("WriteAt() returned %d bytes written, want %d", n, len(tt.data))
				}
			}
		})
	}
}

func TestFileWriter_WriteAt_ClosedWriter(t *testing.T) {
	config := &Config{
		MaxFileSize: 1024,
	}
	
	handler := NewSFTPHandler(config, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	
	upload := &FileUpload{
		data: make([]byte, 0, config.MaxFileSize),
	}

	writer := &FileWriter{
		upload:  upload,
		handler: handler,
		logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		closed:  true, // Mark as closed
	}

	_, err := writer.WriteAt([]byte("test"), 0)
	if err != os.ErrClosed {
		t.Errorf("WriteAt() on closed writer = %v, want %v", err, os.ErrClosed)
	}
}