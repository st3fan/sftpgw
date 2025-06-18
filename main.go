package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	config, err := LoadConfig()
	if err != nil {
		logger.Error("failed to load configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("starting SFTP server", 
		slog.Int("port", config.ServerPort),
		slog.String("virtual_dir", config.VirtualDir),
		slog.Int64("max_file_size", config.MaxFileSize),
		slog.String("s3_bucket", config.S3Bucket),
		slog.String("required_account_id", config.RequiredAccountID),
	)

	server := &SFTPServer{
		config: config,
		logger: logger,
	}

	if err := server.Run(); err != nil {
		logger.Error("server failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

type SFTPServer struct {
	config     *Config
	logger     *slog.Logger
	listener   net.Listener
	sshConfig  *ssh.ServerConfig
	uploader   *S3Uploader
	handler    *SFTPHandler
	auth       *Authenticator
	activeConns sync.WaitGroup
}

func (s *SFTPServer) Run() error {
	if err := s.setupSSHConfig(); err != nil {
		return fmt.Errorf("failed to setup SSH config: %w", err)
	}

	s.uploader = NewS3Uploader(s.config.S3Bucket, s.config.S3BucketPrefix, s.config.S3Region, s.logger)
	s.handler = NewSFTPHandler(s.config, s.uploader, s.logger)
	s.auth = NewAuthenticator(s.config.RequiredAccountID, s.config.S3Region, s.logger)

	s.sshConfig.PasswordCallback = s.auth.Authenticate

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.config.ServerPort))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.config.ServerPort, err)
	}
	s.listener = listener

	s.logger.Info("SFTP server listening", slog.String("address", listener.Addr().String()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.handleSignals(cancel)

	go s.acceptConnections(ctx)

	<-ctx.Done()
	s.logger.Info("shutting down server")

	s.listener.Close()
	s.activeConns.Wait()

	s.logger.Info("server shutdown complete")
	return nil
}

func (s *SFTPServer) setupSSHConfig() error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	privateKeyBytes := pem.EncodeToMemory(privateKeyPEM)

	signer, err := ssh.ParsePrivateKey(privateKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	s.sshConfig = &ssh.ServerConfig{
		MaxAuthTries:      3,
		PasswordCallback:  nil, // Will be set later
		ServerVersion:     "SSH-2.0-SFTPGW",
	}

	s.sshConfig.AddHostKey(signer)
	return nil
}

func (s *SFTPServer) acceptConnections(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				s.logger.Error("failed to accept connection", slog.String("error", err.Error()))
				continue
			}
		}

		s.activeConns.Add(1)
		go s.handleConnection(ctx, conn)
	}
}

func (s *SFTPServer) handleConnection(ctx context.Context, conn net.Conn) {
	defer s.activeConns.Done()
	defer conn.Close()

	clientIP := getClientIP(conn.RemoteAddr())

	conn.SetDeadline(time.Now().Add(s.config.ConnectionTimeout))

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		s.logger.Warn("SSH handshake failed", 
			slog.String("remote_ip", clientIP),
			slog.String("error", err.Error()),
		)
		return
	}
	defer sshConn.Close()

	conn.SetDeadline(time.Time{})

	s.logger.Info("SSH connection established", 
		slog.String("remote_ip", clientIP),
		slog.String("user", sshConn.User()),
		slog.String("client_version", string(sshConn.ClientVersion())),
	)

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			s.logger.Error("failed to accept channel", 
				slog.String("remote_ip", clientIP),
				slog.String("error", err.Error()),
			)
			continue
		}

		go s.handleChannel(ctx, channel, requests, sshConn)
	}
}

func (s *SFTPServer) handleChannel(ctx context.Context, channel ssh.Channel, requests <-chan *ssh.Request, sshConn *ssh.ServerConn) {
	defer channel.Close()

	clientIP := getClientIP(sshConn.RemoteAddr())

	for req := range requests {
		switch req.Type {
		case "subsystem":
			if string(req.Payload[4:]) == "sftp" {
				req.Reply(true, nil)
				s.handleSFTP(ctx, channel, sshConn)
				return
			}
			req.Reply(false, nil)
		default:
			req.Reply(false, nil)
		}
	}

	s.logger.Info("SSH channel closed", slog.String("remote_ip", clientIP))
}

func (s *SFTPServer) handleSFTP(ctx context.Context, channel ssh.Channel, sshConn *ssh.ServerConn) {
	clientIP := getClientIP(sshConn.RemoteAddr())
	
	permissions := sshConn.Permissions
	if permissions == nil {
		s.logger.Error("no permissions found in SSH connection", slog.String("remote_ip", clientIP))
		return
	}

	accessKeyID := permissions.Extensions["aws_access_key_id"]
	secretAccessKey := permissions.Extensions["aws_secret_access_key"]
	accountID := permissions.Extensions["aws_account_id"]

	s.logger.Info("SFTP session started", 
		slog.String("remote_ip", clientIP),
		slog.String("access_key_id", accessKeyID),
		slog.String("account_id", accountID),
	)

	// Create a custom handler for this session with context
	sessionHandler := &SessionSFTPHandler{
		handler:         s.handler,
		clientIP:        clientIP,
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
		accountID:       accountID,
	}

	server := sftp.NewRequestServer(channel, sftp.Handlers{
		FileGet:  sessionHandler,
		FilePut:  sessionHandler,
		FileCmd:  sessionHandler,
		FileList: sessionHandler,
	})

	if err := server.Serve(); err != nil {
		s.logger.Info("SFTP session ended", 
			slog.String("remote_ip", clientIP),
			slog.String("error", err.Error()),
		)
	} else {
		s.logger.Info("SFTP session ended normally", slog.String("remote_ip", clientIP))
	}
}

type SessionSFTPHandler struct {
	handler         *SFTPHandler
	clientIP        string
	accessKeyID     string
	secretAccessKey string
	accountID       string
}

func (h *SessionSFTPHandler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	return h.handler.Fileread(r)
}

func (h *SessionSFTPHandler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	// Create file upload with session context
	upload := &FileUpload{
		data:      make([]byte, 0, h.handler.config.MaxFileSize),
		path:      r.Filepath,
		clientIP:  h.clientIP,
		accessKey: h.accessKeyID,
		secretKey: h.secretAccessKey,
	}

	if !h.handler.isPathAllowed(r.Filepath) {
		h.handler.logger.Warn("file write rejected: path not allowed", 
			slog.String("remote_ip", h.clientIP),
			slog.String("access_key_id", h.accessKeyID),
			slog.String("file_path", r.Filepath),
		)
		return nil, os.ErrPermission
	}

	h.handler.logger.Info("file write request",
		slog.String("remote_ip", h.clientIP),
		slog.String("access_key_id", h.accessKeyID),
		slog.String("file_path", r.Filepath),
	)

	h.handler.activeUploads.Store(r.Filepath, upload)

	return &FileWriter{
		upload:  upload,
		handler: h.handler,
		logger:  h.handler.logger,
	}, nil
}

func (h *SessionSFTPHandler) Filecmd(r *sftp.Request) error {
	logCtx := slog.Group("file_cmd",
		"remote_ip", h.clientIP,
		"access_key_id", h.accessKeyID,
		"file_path", r.Filepath,
		"method", r.Method,
	)

	switch r.Method {
	case "Remove":
		h.handler.logger.Warn("file remove rejected: operation not allowed", logCtx)
		return os.ErrPermission
	case "Rename":
		h.handler.logger.Warn("file rename rejected: operation not allowed", logCtx)
		return os.ErrPermission
	case "Mkdir":
		h.handler.logger.Info("mkdir request (virtual operation)", logCtx)
		return nil
	case "Rmdir":
		h.handler.logger.Warn("rmdir rejected: operation not allowed", logCtx)
		return os.ErrPermission
	case "Setstat":
		h.handler.logger.Info("setstat request (ignored)", logCtx)
		return nil
	default:
		h.handler.logger.Warn("unknown file command rejected", logCtx)
		return os.ErrInvalid
	}
}

func (h *SessionSFTPHandler) Fileinfo(r *sftp.Request) ([]os.FileInfo, error) {
	return nil, os.ErrPermission
}

func (h *SessionSFTPHandler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	return nil, os.ErrPermission
}

func (s *SFTPServer) handleSignals(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	s.logger.Info("received signal", slog.String("signal", sig.String()))
	cancel()
}

