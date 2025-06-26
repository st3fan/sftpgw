package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

type SFTPHandler struct {
	config     *Config
	uploader   *S3Uploader
	logger     *slog.Logger
	activeUploads sync.Map // track active file uploads
}

type FileUpload struct {
	data      []byte
	path      string
	clientIP  string
	accessKey string
	secretKey string
	mu        sync.Mutex
}

func NewSFTPHandler(config *Config, uploader *S3Uploader, logger *slog.Logger) *SFTPHandler {
	return &SFTPHandler{
		config:   config,
		uploader: uploader,
		logger:   logger,
	}
}

func (h *SFTPHandler) Fileread(*sftp.Request) (io.ReaderAt, error) {
	return nil, os.ErrPermission // read operations not allowed
}

func (h *SFTPHandler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	clientIP, _ := r.Context().Value("client_ip").(string)
	accessKey, _ := r.Context().Value("access_key_id").(string)
	secretKey, _ := r.Context().Value("secret_access_key").(string)

	logCtx := slog.Group("file_write",
		"remote_ip", clientIP,
		"access_key_id", accessKey,
		"file_path", r.Filepath,
	)

	if !h.isPathAllowed(r.Filepath) {
		h.logger.Warn("file write rejected: path not allowed", logCtx)
		return nil, os.ErrPermission
	}

	h.logger.Info("file write request", logCtx)

	upload := &FileUpload{
		data:      make([]byte, 0, h.config.MaxFileSize),
		path:      r.Filepath,
		clientIP:  clientIP,
		accessKey: accessKey,
		secretKey: secretKey,
	}

	h.activeUploads.Store(r.Filepath, upload)

	return &FileWriter{
		upload:  upload,
		handler: h,
		logger:  h.logger,
	}, nil
}

func (h *SFTPHandler) Filecmd(r *sftp.Request) error {
	clientIP, _ := r.Context().Value("client_ip").(string)
	accessKey, _ := r.Context().Value("access_key_id").(string)

	logCtx := slog.Group("file_cmd",
		"remote_ip", clientIP,
		"access_key_id", accessKey,
		"file_path", r.Filepath,
		"method", r.Method,
	)

	switch r.Method {
	case "Remove":
		h.logger.Warn("file remove rejected: operation not allowed", logCtx)
		return os.ErrPermission
	case "Rename":
		h.logger.Warn("file rename rejected: operation not allowed", logCtx)
		return os.ErrPermission
	case "Mkdir":
		h.logger.Info("mkdir request (virtual operation)", logCtx)
		return nil // Allow mkdir for client compatibility but don't actually create directories
	case "Rmdir":
		h.logger.Warn("rmdir rejected: operation not allowed", logCtx)
		return os.ErrPermission
	case "Setstat":
		h.logger.Info("setstat request (ignored)", logCtx)
		return nil // Ignore setstat requests for compatibility
	default:
		h.logger.Warn("unknown file command rejected", logCtx)
		return os.ErrInvalid
	}
}

func (h *SFTPHandler) Fileinfo(r *sftp.Request) ([]os.FileInfo, error) {
	return nil, os.ErrPermission // no directory listing allowed
}

func (h *SFTPHandler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	return nil, os.ErrPermission // no directory listing allowed
}

func (h *SFTPHandler) isPathAllowed(path string) bool {
	// isPathAllowed checks if the given path is allowed for file operations.
	// It ensures that:
	// 1. The path, after cleaning, is absolute.
	// 2. The path does not attempt to traverse outside the configured VirtualDir.
	//    This is achieved by checking if the relative path from VirtualDir starts with "..".
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		cleanPath = "/" + cleanPath
	}

	relPath, err := filepath.Rel(h.config.VirtualDir, cleanPath)
	if err != nil {
		return false
	}

	return !strings.HasPrefix(relPath, "..")
}

type FileWriter struct {
	upload  *FileUpload
	handler *SFTPHandler
	logger  *slog.Logger
	closed  bool
}

func (fw *FileWriter) WriteAt(p []byte, off int64) (int, error) {
	fw.upload.mu.Lock()
	defer fw.upload.mu.Unlock()

	if fw.closed {
		return 0, os.ErrClosed
	}

	logCtx := slog.Group("file_write_at",
		"remote_ip", fw.upload.clientIP,
		"access_key_id", fw.upload.accessKey,
		"file_path", fw.upload.path,
		"offset", off,
		"length", len(p),
		"current_size", len(fw.upload.data),
	)

	endPos := off + int64(len(p))
	if endPos > fw.handler.config.MaxFileSize {
		fw.logger.Warn("file write rejected: exceeds size limit", logCtx,
			slog.Int64("max_size", fw.handler.config.MaxFileSize),
			slog.Int64("attempted_size", endPos),
		)
		return 0, fmt.Errorf("file too large")
	}

	if int64(len(fw.upload.data)) < endPos {
		newData := make([]byte, endPos)
		copy(newData, fw.upload.data)
		fw.upload.data = newData
	}

	copy(fw.upload.data[off:], p)

	fw.logger.Debug("file data written", logCtx, slog.Int("bytes_written", len(p)))

	return len(p), nil
}

func (fw *FileWriter) Close() error {
	fw.upload.mu.Lock()
	defer fw.upload.mu.Unlock()

	if fw.closed {
		return nil
	}
	fw.closed = true

	defer fw.handler.activeUploads.Delete(fw.upload.path)

	logCtx := slog.Group("file_close",
		"remote_ip", fw.upload.clientIP,
		"access_key_id", fw.upload.accessKey,
		"file_path", fw.upload.path,
		"final_size", len(fw.upload.data),
	)

	fw.logger.Info("file upload completed, starting S3 upload", logCtx)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	err := fw.handler.uploader.UploadFile(
		ctx,
		fw.upload.accessKey,
		fw.upload.secretKey,
		fw.upload.clientIP,
		fw.upload.path,
		fw.upload.data,
	)

	if err != nil {
		fw.logger.Error("S3 upload failed", logCtx, slog.String("error", err.Error()))
		return fmt.Errorf("upload failed: %w", err)
	}

	fw.logger.Info("file upload successful", logCtx)
	return nil
}