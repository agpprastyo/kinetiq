package export

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockS3Client struct {
	mock.Mock
}

func (m *mockS3Client) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.HeadBucketOutput), args.Error(1)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.PutObjectOutput), args.Error(1)
}

func TestStorageDriver_TestConnection(t *testing.T) {
	ctx := context.Background()

	t.Run("success connection", func(t *testing.T) {
		mockClient := new(mockS3Client)
		mockClient.On("HeadBucket", ctx, mock.MatchedBy(func(p *s3.HeadBucketInput) bool {
			return *p.Bucket == "my-bucket"
		})).Return(&s3.HeadBucketOutput{}, nil)

		driver := NewS3StorageDriverWithClient(mockClient)
		cfg := StorageConfig{
			Provider:        "s3",
			BucketName:      "my-bucket",
			Region:          "us-east-1",
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
		}

		err := driver.TestConnection(ctx, cfg)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("missing required fields", func(t *testing.T) {
		driver := NewS3StorageDriver()
		err := driver.TestConnection(ctx, StorageConfig{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "bucket_name is required")
	})

	t.Run("permission denied error", func(t *testing.T) {
		mockClient := new(mockS3Client)
		mockClient.On("HeadBucket", ctx, mock.Anything).Return(nil, errors.New("403 Forbidden: Invalid credentials"))

		driver := NewS3StorageDriverWithClient(mockClient)
		cfg := StorageConfig{
			Provider:        "s3",
			BucketName:      "protected-bucket",
			Region:          "us-east-1",
			AccessKeyID:     "bad-key",
			SecretAccessKey: "bad-secret",
		}

		err := driver.TestConnection(ctx, cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "403 Forbidden")
		mockClient.AssertExpectations(t)
	})
}

func TestStorageDriver_UploadGzip(t *testing.T) {
	ctx := context.Background()

	t.Run("successful upload with prefix", func(t *testing.T) {
		mockClient := new(mockS3Client)
		mockClient.On("PutObject", ctx, mock.MatchedBy(func(p *s3.PutObjectInput) bool {
			return *p.Bucket == "my-bucket" &&
				*p.Key == "kinetiq-events/year=2026/events.jsonl.gz" &&
				*p.ContentType == "application/gzip"
		})).Return(&s3.PutObjectOutput{}, nil)

		driver := NewS3StorageDriverWithClient(mockClient)
		cfg := StorageConfig{
			BucketName: "my-bucket",
			PrefixPath: "kinetiq-events",
		}

		err := driver.UploadGzip(ctx, cfg, "year=2026/events.jsonl.gz", strings.NewReader("dummy compressed data"))
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
}
