package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"golang.org/x/crypto/ssh"
)

type Authenticator struct {
	requiredAccountID string
	region            string
	logger            *slog.Logger
}

func NewAuthenticator(requiredAccountID, region string, logger *slog.Logger) *Authenticator {
	return &Authenticator{
		requiredAccountID: requiredAccountID,
		region:            region,
		logger:            logger,
	}
}

func (a *Authenticator) Authenticate(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	remoteAddr := conn.RemoteAddr()
	clientIP := getClientIP(remoteAddr)
	accessKeyID := conn.User()

	logCtx := slog.Group("auth",
		"remote_ip", clientIP,
		"access_key_id", accessKeyID,
	)

	a.logger.Info("authentication attempt", logCtx)

	secretAccessKey := string(password)
	if accessKeyID == "" || secretAccessKey == "" {
		a.logger.Warn("authentication failed: empty credentials", logCtx)
		return nil, fmt.Errorf("credentials cannot be empty")
	}

	configOptions := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyID,
			secretAccessKey,
			"",
		)),
	}

	if a.region != "" {
		configOptions = append(configOptions, config.WithRegion(a.region))
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), configOptions...)
	if err != nil {
		a.logger.Error("failed to load AWS config", logCtx, slog.String("error", err.Error()))
		return nil, fmt.Errorf("invalid credentials")
	}

	stsClient := sts.NewFromConfig(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		a.logger.Warn("STS GetCallerIdentity failed", logCtx, slog.String("error", err.Error()))
		return nil, fmt.Errorf("invalid credentials")
	}

	if result.Account == nil {
		a.logger.Error("STS GetCallerIdentity returned nil account", logCtx)
		return nil, fmt.Errorf("invalid credentials")
	}

	accountID := *result.Account
	if accountID != a.requiredAccountID {
		a.logger.Warn("authentication failed: wrong account ID",
			logCtx,
			slog.String("actual_account_id", accountID),
			slog.String("required_account_id", a.requiredAccountID),
		)
		return nil, fmt.Errorf("unauthorized account")
	}

	a.logger.Info("authentication successful",
		logCtx,
		slog.String("account_id", accountID),
		slog.String("user_id", aws.ToString(result.UserId)),
		slog.String("arn", aws.ToString(result.Arn)),
	)

	return &ssh.Permissions{
		Extensions: map[string]string{
			"aws_access_key_id":     accessKeyID,
			"aws_secret_access_key": secretAccessKey,
			"aws_account_id":        accountID,
			"client_ip":             clientIP,
		},
	}, nil
}

func getClientIP(addr net.Addr) string {
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		return tcpAddr.IP.String()
	}
	return addr.String()
}