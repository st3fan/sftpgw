package main

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/pkg/sftp"
)

func TestSFTPHandler_Filecmd(t *testing.T) {
	config := &Config{
		VirtualDir: "/uploads",
	}
	
	handler := NewSFTPHandler(config, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	tests := []struct {
		name        string
		method      string
		filepath    string
		expectError bool
		expectedErr error
	}{
		{
			name:        "Remove operation should be rejected",
			method:      "Remove",
			filepath:    "/uploads/test.txt",
			expectError: true,
			expectedErr: os.ErrPermission,
		},
		{
			name:        "Rename operation should be rejected",
			method:      "Rename", 
			filepath:    "/uploads/test.txt",
			expectError: true,
			expectedErr: os.ErrPermission,
		},
		{
			name:        "Mkdir operation should be allowed",
			method:      "Mkdir",
			filepath:    "/uploads/newfolder",
			expectError: false,
		},
		{
			name:        "Rmdir operation should be rejected",
			method:      "Rmdir",
			filepath:    "/uploads/folder",
			expectError: true,
			expectedErr: os.ErrPermission,
		},
		{
			name:        "Setstat operation should be allowed (ignored)",
			method:      "Setstat",
			filepath:    "/uploads/test.txt",
			expectError: false,
		},
		{
			name:        "Unknown operation should be rejected",
			method:      "UnknownOperation",
			filepath:    "/uploads/test.txt",
			expectError: true,
			expectedErr: os.ErrInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a context with mock values
			ctx := context.WithValue(context.Background(), "client_ip", "127.0.0.1")
			ctx = context.WithValue(ctx, "access_key_id", "test-key")
			
			request := &sftp.Request{
				Method:   tt.method,
				Filepath: tt.filepath,
			}
			request = request.WithContext(ctx)
			
			err := handler.Filecmd(request)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Filecmd() expected error but got none")
				} else if tt.expectedErr != nil && err != tt.expectedErr {
					t.Errorf("Filecmd() error = %v, want %v", err, tt.expectedErr)
				}
			} else {
				if err != nil {
					t.Errorf("Filecmd() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSFTPHandler_Fileread(t *testing.T) {
	config := &Config{
		VirtualDir: "/uploads",
	}
	
	handler := NewSFTPHandler(config, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	request := &sftp.Request{
		Method:   "Get",
		Filepath: "/uploads/test.txt",
	}

	reader, err := handler.Fileread(request)
	
	if err != os.ErrPermission {
		t.Errorf("Fileread() error = %v, want %v", err, os.ErrPermission)
	}
	
	if reader != nil {
		t.Errorf("Fileread() returned non-nil reader, expected nil")
	}
}

func TestSFTPHandler_Fileinfo(t *testing.T) {
	config := &Config{
		VirtualDir: "/uploads",
	}
	
	handler := NewSFTPHandler(config, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	request := &sftp.Request{
		Method:   "Stat",
		Filepath: "/uploads/test.txt",
	}

	fileInfo, err := handler.Fileinfo(request)
	
	if err != os.ErrPermission {
		t.Errorf("Fileinfo() error = %v, want %v", err, os.ErrPermission)
	}
	
	if fileInfo != nil {
		t.Errorf("Fileinfo() returned non-nil fileInfo, expected nil")
	}
}

func TestSFTPHandler_Filelist(t *testing.T) {
	config := &Config{
		VirtualDir: "/uploads",
	}
	
	handler := NewSFTPHandler(config, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	request := &sftp.Request{
		Method:   "List",
		Filepath: "/uploads/",
	}

	lister, err := handler.Filelist(request)
	
	if err != os.ErrPermission {
		t.Errorf("Filelist() error = %v, want %v", err, os.ErrPermission)
	}
	
	if lister != nil {
		t.Errorf("Filelist() returned non-nil lister, expected nil") 
	}
}

func TestSessionSFTPHandler_Filecmd(t *testing.T) {
	config := &Config{
		VirtualDir: "/uploads",
	}
	
	baseHandler := NewSFTPHandler(config, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	
	sessionHandler := &SessionSFTPHandler{
		handler:         baseHandler,
		clientIP:        "192.168.1.100",
		accessKeyID:     "AKIATEST123",
		secretAccessKey: "secret123",
		accountID:       "123456789012",
	}

	tests := []struct {
		name        string
		method      string
		filepath    string
		expectError bool
		expectedErr error
	}{
		{
			name:        "Remove operation should be rejected",
			method:      "Remove",
			filepath:    "/uploads/test.txt",
			expectError: true,
			expectedErr: os.ErrPermission,
		},
		{
			name:        "Mkdir operation should succeed",
			method:      "Mkdir",
			filepath:    "/uploads/newfolder",
			expectError: false,
		},
		{
			name:        "Setstat operation should succeed",
			method:      "Setstat", 
			filepath:    "/uploads/test.txt",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &sftp.Request{
				Method:   tt.method,
				Filepath: tt.filepath,
			}
			
			err := sessionHandler.Filecmd(request)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("SessionSFTPHandler.Filecmd() expected error but got none")
				} else if tt.expectedErr != nil && err != tt.expectedErr {
					t.Errorf("SessionSFTPHandler.Filecmd() error = %v, want %v", err, tt.expectedErr)
				}
			} else {
				if err != nil {
					t.Errorf("SessionSFTPHandler.Filecmd() unexpected error: %v", err)
				}
			}
		})
	}
}