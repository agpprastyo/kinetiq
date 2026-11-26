package export

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"kinetiq/internal/webhook"
)

type StorageConfig struct {
	Provider        string `json:"provider"` // s3, r2, gcs, minio
	BucketName      string `json:"bucket_name"`
	EndpointURL     string `json:"endpoint_url,omitempty"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	PrefixPath      string `json:"prefix_path"`
}

// StorageDriver defines client operations for S3-compatible cloud storage sync
type StorageDriver interface {
	TestConnection(ctx context.Context, cfg StorageConfig) error
	UploadGzip(ctx context.Context, cfg StorageConfig, key string, data io.Reader) error
}

type S3StorageDriver struct {
	// clientOverride allows injecting mock or preconfigured client for testing
	clientOverride S3APIClient
}

// S3APIClient abstracts the AWS S3 client operations needed
type S3APIClient interface {
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func NewS3StorageDriver() *S3StorageDriver {
	return &S3StorageDriver{}
}

func NewS3StorageDriverWithClient(client S3APIClient) *S3StorageDriver {
	return &S3StorageDriver{clientOverride: client}
}

func (d *S3StorageDriver) getClient(ctx context.Context, cfg StorageConfig) (S3APIClient, error) {
	if d.clientOverride != nil {
		return d.clientOverride, nil
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	optFns := []func(*awsConfig.LoadOptions) error{
		awsConfig.WithRegion(region),
		awsConfig.WithHTTPClient(webhook.NewSafeHTTPClient(2 * time.Minute)),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
	}

	awsCfg, err := awsConfig.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var s3OptFns []func(*s3.Options)
	endpoint := strings.TrimSpace(cfg.EndpointURL)
	if endpoint != "" {
		s3OptFns = append(s3OptFns, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			// Force path style for minio and non-AWS endpoints
			if cfg.Provider == "minio" || strings.Contains(endpoint, "localhost") || strings.Contains(endpoint, "127.0.0.1") {
				o.UsePathStyle = true
			}
		})
	}

	client := s3.NewFromConfig(awsCfg, s3OptFns...)
	return client, nil
}

// validateStorageConfig centralizes required-field checks for storage configs.
func validateStorageConfig(cfg StorageConfig, requireCredentials bool) error {
	if cfg.BucketName == "" {
		return fmt.Errorf("bucket_name is required")
	}
	if requireCredentials && (cfg.AccessKeyID == "" || cfg.SecretAccessKey == "") {
		return fmt.Errorf("access_key_id and secret_access_key are required")
	}
	return nil
}

func (d *S3StorageDriver) TestConnection(ctx context.Context, cfg StorageConfig) error {
	if err := validateStorageConfig(cfg, true); err != nil {
		return err
	}

	client, err := d.getClient(ctx, cfg)
	if err != nil {
		return err
	}

	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(cfg.BucketName),
	})
	if err != nil {
		return fmt.Errorf("failed to verify bucket access: %w", err)
	}

	return nil
}

func (d *S3StorageDriver) UploadGzip(ctx context.Context, cfg StorageConfig, key string, data io.Reader) error {
	if err := validateStorageConfig(cfg, false); err != nil {
		return err
	}

	client, err := d.getClient(ctx, cfg)
	if err != nil {
		return err
	}

	cleanKey := strings.TrimPrefix(key, "/")
	if cfg.PrefixPath != "" {
		prefix := strings.Trim(cfg.PrefixPath, "/")
		if !strings.HasPrefix(cleanKey, prefix) {
			cleanKey = prefix + "/" + cleanKey
		}
	}

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:          aws.String(cfg.BucketName),
		Key:             aws.String(cleanKey),
		Body:            data,
		ContentType:     aws.String("application/gzip"),
		ContentEncoding: aws.String("gzip"),
	})
	if err != nil {
		return fmt.Errorf("failed to upload gzip to storage: %w", err)
	}

	return nil
}
